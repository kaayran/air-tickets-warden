// Package bot is the Telegram bot: the identity entry point (/start with an
// "Open App" web_app button) and — from Phase 3 — the notification channel.
// It uses long polling, so it needs no inbound tunnel; only the Mini App does.
package bot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const openAppText = "Open App"

// Bot wraps the go-telegram client with this service's handlers and access
// control.
type Bot struct {
	api       *bot.Bot
	log       *slog.Logger
	publicURL string
	allowed   map[int64]struct{}
}

// New builds a Bot restricted to the given whitelist of chat ids. The web_app
// buttons point at publicURL (resolved at runtime, so a fresh dev tunnel URL
// needs only a restart — no BotFather edits).
func New(token, publicURL string, allowedUserIDs []int64, log *slog.Logger) (*Bot, error) {
	allowed := make(map[int64]struct{}, len(allowedUserIDs))
	for _, id := range allowedUserIDs {
		allowed[id] = struct{}{}
	}

	b := &Bot{log: log, publicURL: publicURL, allowed: allowed}

	opts := []bot.Option{
		bot.WithMiddlewares(b.recoverMiddleware, b.whitelistMiddleware),
		bot.WithDefaultHandler(b.handleHelp),
	}
	api, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}
	api.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, b.handleStart)
	api.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypeExact, b.handleHelp)
	b.api = api
	return b, nil
}

// Start configures the chat menu button and blocks polling for updates until
// ctx is cancelled.
func (b *Bot) Start(ctx context.Context) {
	b.configureMenuButton(ctx)
	b.log.Info("bot polling started")
	b.api.Start(ctx) // returns when ctx is cancelled
	b.log.Info("bot polling stopped")
}

// configureMenuButton points the chat menu button at the Mini App so users can
// open it without scrolling to the /start message.
func (b *Bot) configureMenuButton(ctx context.Context) {
	_, err := b.api.SetChatMenuButton(ctx, &bot.SetChatMenuButtonParams{
		MenuButton: models.MenuButtonWebApp{
			Type:   "web_app",
			Text:   openAppText,
			WebApp: models.WebAppInfo{URL: b.publicURL},
		},
	})
	if err != nil {
		// Non-fatal: the /start keyboard button still opens the app.
		b.log.Warn("set chat menu button failed", "err", err)
	}
}

// send wraps SendMessage, logging (rather than dropping) delivery errors.
func (b *Bot) send(ctx context.Context, api *bot.Bot, params *bot.SendMessageParams) {
	if _, err := api.SendMessage(ctx, params); err != nil {
		b.log.Warn("send message failed", "chat_id", params.ChatID, "err", err)
	}
}

// recoverMiddleware is the outermost guard: a panic in any handler is logged
// instead of taking down the whole process with it.
func (b *Bot) recoverMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, api *bot.Bot, update *models.Update) {
		defer func() {
			if r := recover(); r != nil {
				b.log.Error("panic while handling update", "recover", r, "update_id", update.ID)
			}
		}()
		next(ctx, api, update)
	}
}

// whitelistMiddleware silently drops any update whose sender is not on the
// whitelist. Deliberately no reply: answering would confirm to strangers that
// the bot is alive and let them trigger outbound API calls. The Warn log is
// also how a new user's id is discovered before adding it to ALLOWED_USER_IDS.
func (b *Bot) whitelistMiddleware(next bot.HandlerFunc) bot.HandlerFunc {
	return func(ctx context.Context, api *bot.Bot, update *models.Update) {
		id, _, ok := senderAndChat(update)
		if !ok {
			return // updates without an identifiable sender are ignored
		}
		if _, allowed := b.allowed[id]; !allowed {
			b.log.Warn("rejected non-whitelisted user", "user_id", id)
			return
		}
		next(ctx, api, update)
	}
}

// handleStart greets the user and shows an "Open App" web_app keyboard button.
func (b *Bot) handleStart(ctx context.Context, api *bot.Bot, update *models.Update) {
	b.send(ctx, api, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Welcome to Air Tickets Warden. Tap the button below to open the app.",
		ReplyMarkup: models.ReplyKeyboardMarkup{
			ResizeKeyboard: true,
			Keyboard: [][]models.KeyboardButton{{
				{Text: openAppText, WebApp: &models.WebAppInfo{URL: b.publicURL}},
			}},
		},
	})
}

// handleHelp is also the default handler for unrecognised messages.
func (b *Bot) handleHelp(ctx context.Context, api *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	b.send(ctx, api, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Air Tickets Warden monitors flight prices and alerts you here.\n\n/start — open the app\n/help — this message",
	})
}

// senderAndChat extracts the sender id and originating chat id from the kinds of
// update the bot handles (messages and callback queries).
func senderAndChat(u *models.Update) (userID, chatID int64, ok bool) {
	switch {
	case u.Message != nil && u.Message.From != nil:
		return u.Message.From.ID, u.Message.Chat.ID, true
	case u.CallbackQuery != nil:
		// CallbackQuery.Message is MaybeInaccessibleMessage: .Message is nil
		// when the source message is older than 48h (Telegram sends an
		// InaccessibleMessage instead) or when the callback comes from an
		// inline-mode message (no chat at all).
		cq := u.CallbackQuery
		switch {
		case cq.Message.Message != nil:
			return cq.From.ID, cq.Message.Message.Chat.ID, true
		case cq.Message.InaccessibleMessage != nil:
			return cq.From.ID, cq.Message.InaccessibleMessage.Chat.ID, true
		default:
			// Inline-mode callback: no chat, but in this private bot the
			// user's private chat id equals their user id.
			return cq.From.ID, cq.From.ID, true
		}
	default:
		return 0, 0, false
	}
}

package bot

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

// TestSenderAndChat covers the callback-query branches that cannot be
// exercised manually in Phase 0: an inaccessible source message (older than
// 48h — the *normal* case for buttons on old alerts) and an inline-mode
// callback with no chat at all. Both used to nil-panic.
func TestSenderAndChat(t *testing.T) {
	tests := []struct {
		name     string
		update   *models.Update
		wantUser int64
		wantChat int64
		wantOK   bool
	}{
		{
			name: "message",
			update: &models.Update{Message: &models.Message{
				From: &models.User{ID: 7},
				Chat: models.Chat{ID: 42},
			}},
			wantUser: 7, wantChat: 42, wantOK: true,
		},
		{
			name:   "message without sender",
			update: &models.Update{Message: &models.Message{Chat: models.Chat{ID: 42}}},
			wantOK: false,
		},
		{
			name: "callback with accessible message",
			update: &models.Update{CallbackQuery: &models.CallbackQuery{
				From: models.User{ID: 7},
				Message: models.MaybeInaccessibleMessage{
					Message: &models.Message{Chat: models.Chat{ID: 42}},
				},
			}},
			wantUser: 7, wantChat: 42, wantOK: true,
		},
		{
			name: "callback with inaccessible message (older than 48h)",
			update: &models.Update{CallbackQuery: &models.CallbackQuery{
				From: models.User{ID: 7},
				Message: models.MaybeInaccessibleMessage{
					InaccessibleMessage: &models.InaccessibleMessage{Chat: models.Chat{ID: 42}},
				},
			}},
			wantUser: 7, wantChat: 42, wantOK: true,
		},
		{
			name: "inline-mode callback without message",
			update: &models.Update{CallbackQuery: &models.CallbackQuery{
				From: models.User{ID: 7},
			}},
			wantUser: 7, wantChat: 7, wantOK: true,
		},
		{
			name:   "empty update",
			update: &models.Update{},
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, chat, ok := senderAndChat(tt.update)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if user != tt.wantUser || chat != tt.wantChat {
				t.Errorf("got (user=%d, chat=%d), want (user=%d, chat=%d)", user, chat, tt.wantUser, tt.wantChat)
			}
		})
	}
}

package subscriptions

import (
	"context"
	"errors"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/kaayran/air-tickets-warden/internal/domain"
	"github.com/kaayran/air-tickets-warden/internal/storage"
)

const (
	ownerChat   int64 = 100
	foreignChat int64 = 200
)

// setup starts a disposable Postgres, applies the real goose migrations, and
// returns a Manager backed by it. Skipped under -short (CI runs unit-only).
func setup(t *testing.T) *Manager {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test needs Docker; run without -short")
	}
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("warden_test"),
		tcpostgres.WithUsername("warden"),
		tcpostgres.WithPassword("warden"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	if err := storage.Migrate(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := storage.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(store.Close)
	return New(store.Queries)
}

func ptr[T any](v T) *T { return &v }

func date(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func newSub(chatID int64) domain.Subscription {
	return domain.Subscription{
		UserChatID:         chatID,
		Origin:             "BEG",
		OriginAlternatives: []string{"BUD", "SOF"},
		Destinations:       []string{"BCN", "MAD"},
		DateFrom:           date(2030, 7, 1),
		DateTo:             date(2030, 7, 15),
		MaxPriceMinor:      ptr[int64](15000),
		AirlinesBlacklist:  []string{"FR"},
		AlertStrategy:      domain.StrategyAbsoluteThreshold,
		Status:             domain.StatusActive,
	}
}

func TestManagerCRUD(t *testing.T) {
	m := setup(t)
	ctx := context.Background()

	created, err := m.Create(ctx, newSub(ownerChat))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("Create returned empty ID")
	}
	if created.Status != domain.StatusActive || created.AlertStrategy != domain.StrategyAbsoluteThreshold {
		t.Errorf("enums did not round-trip: %+v", created)
	}
	if !created.DateFrom.Equal(date(2030, 7, 1)) || !created.DateTo.Equal(date(2030, 7, 15)) {
		t.Errorf("dates did not round-trip: %v .. %v", created.DateFrom, created.DateTo)
	}
	if len(created.OriginAlternatives) != 2 || len(created.Destinations) != 2 || len(created.AirlinesBlacklist) != 1 {
		t.Errorf("arrays did not round-trip: %+v", created)
	}
	if created.MaxPriceMinor == nil || *created.MaxPriceMinor != 15000 {
		t.Errorf("max price did not round-trip: %v", created.MaxPriceMinor)
	}
	if created.NextCheckAt.IsZero() || created.CreatedAt.IsZero() {
		t.Error("DB-side defaults (next_check_at/created_at) missing")
	}

	t.Run("get is ownership-scoped", func(t *testing.T) {
		got, err := m.Get(ctx, ownerChat, created.ID)
		if err != nil {
			t.Fatalf("owner Get: %v", err)
		}
		if got.ID != created.ID {
			t.Errorf("Get returned wrong row: %s", got.ID)
		}
		if _, err := m.Get(ctx, foreignChat, created.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("foreign Get: err = %v, want ErrNotFound", err)
		}
		if _, err := m.Get(ctx, ownerChat, "not-a-uuid"); !errors.Is(err, ErrNotFound) {
			t.Errorf("malformed id: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("list isolates users", func(t *testing.T) {
		if _, err := m.Create(ctx, newSub(foreignChat)); err != nil {
			t.Fatalf("Create for foreign user: %v", err)
		}
		mine, err := m.List(ctx, ownerChat)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(mine) != 1 || mine[0].UserChatID != ownerChat {
			t.Errorf("owner list = %d rows, want exactly own 1", len(mine))
		}
	})

	t.Run("update writes and stays scoped", func(t *testing.T) {
		mod := created
		mod.Status = domain.StatusPaused
		mod.MaxPriceMinor = ptr[int64](9900)
		mod.MutedUntil = ptr(time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC))
		updated, err := m.Update(ctx, mod)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		if updated.Status != domain.StatusPaused || *updated.MaxPriceMinor != 9900 {
			t.Errorf("update did not stick: %+v", updated)
		}
		if updated.MutedUntil == nil || !updated.MutedUntil.Equal(*mod.MutedUntil) {
			t.Errorf("muted_until did not round-trip: %v", updated.MutedUntil)
		}
		if !updated.UpdatedAt.After(created.UpdatedAt) {
			t.Error("updated_at not bumped")
		}

		hijack := created
		hijack.UserChatID = foreignChat
		hijack.Status = domain.StatusArchived
		if _, err := m.Update(ctx, hijack); !errors.Is(err, ErrNotFound) {
			t.Errorf("foreign Update: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("delete is scoped and idempotence reads as not found", func(t *testing.T) {
		victim, err := m.Create(ctx, newSub(ownerChat))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if err := m.Delete(ctx, foreignChat, victim.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("foreign Delete: err = %v, want ErrNotFound", err)
		}
		if _, err := m.Get(ctx, ownerChat, victim.ID); err != nil {
			t.Errorf("row vanished after foreign delete attempt: %v", err)
		}
		if err := m.Delete(ctx, ownerChat, victim.ID); err != nil {
			t.Fatalf("owner Delete: %v", err)
		}
		if err := m.Delete(ctx, ownerChat, victim.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("second Delete: err = %v, want ErrNotFound", err)
		}
	})
}

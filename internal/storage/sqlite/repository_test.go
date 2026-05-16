package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestMessageRepositoryCreateGetAndListInbox(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewMessageRepository(db)

	msg := domain.NewMessage(domain.DirectionInbound, "hello", "telegram")
	msg.Channels = []string{"telegram"}
	msg.Metadata = map[string]any{"chat_id": "42"}
	msg.PlatformMessageIDs = map[string]string{"telegram": "abc"}

	if err := repo.Create(ctx, msg); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, msg.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ContentOriginal != "hello" || got.Metadata["chat_id"] != "42" {
		t.Fatalf("unexpected message: %+v", got)
	}

	items, err := repo.ListInbox(ctx, MessageListOptions{Channel: "telegram", Limit: 10})
	if err != nil {
		t.Fatalf("ListInbox: %v", err)
	}
	if len(items) != 1 || items[0].ID != msg.ID {
		t.Fatalf("ListInbox returned %+v", items)
	}
}

func TestChannelRepositoryStoresJSONText(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewChannelRepository(db)

	ch := domain.NewChannel(domain.PlatformTelegram, "ops")
	ch.Config = map[string]any{"bot_token": "encrypted", "chat_id": "42"}
	ch.Rules = []domain.Rule{{Type: domain.RuleTypeKeyword, Pattern: "urgent", Channels: []string{"telegram"}}}

	if err := repo.Create(ctx, ch); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, ch.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Config["bot_token"] != "encrypted" || len(got.Rules) != 1 {
		t.Fatalf("unexpected channel: %+v", got)
	}
}

func TestWebhookRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewWebhookRepository(db)

	hook := domain.NewWebhookConfig("https://example.test/hook", []string{"inbound.telegram"})
	hook.Secret = "encrypted-secret"
	if err := repo.Create(ctx, hook); err != nil {
		t.Fatalf("Create: %v", err)
	}

	hooks, err := repo.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(hooks) != 1 || hooks[0].Secret != "encrypted-secret" {
		t.Fatalf("unexpected hooks: %+v", hooks)
	}
}

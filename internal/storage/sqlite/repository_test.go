package sqlite

import (
	"context"
	"database/sql"
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

func TestACPOutboxRepositoryCreateGetAndStatusTransitions(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewACPOutboxRepository(db)

	event := &domain.ACPEvent{
		Version:   domain.ACPEventVersion,
		EventType: domain.ACPEventTypeChannelInboundAnalyzed,
		MessageID: "msg_1",
		Source:    domain.ACPEventSource{Platform: "telegram", ChannelID: "-100"},
		Routing:   domain.ACPRouting{ShouldForward: true, Project: "notification", Agent: "triage", Priority: "normal", Confidence: 0.9},
		Analysis:  domain.ACPAnalysis{Intent: "docs_request", Summary: "summary", Action: "action", Entities: []string{"README"}, Language: "en"},
		Content:   domain.ACPContent{Original: "hello", Normalized: "hello"},
	}
	item := domain.NewACPOutboxItem("msg_1", event, `{"raw":true}`)
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Event == nil || got.Event.MessageID != "msg_1" || got.Status != domain.ACPOutboxStatusPending {
		t.Fatalf("unexpected item: %+v", got)
	}
	if got.EventJSON == "" || got.RawLLMOutput != `{"raw":true}` {
		t.Fatalf("unexpected json/raw: event=%q raw=%q", got.EventJSON, got.RawLLMOutput)
	}

	if err := repo.MarkSkipped(ctx, item.ID, "low_confidence"); err != nil {
		t.Fatalf("MarkSkipped: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != domain.ACPOutboxStatusSkipped || got.SkipReason != "low_confidence" {
		t.Fatalf("unexpected skipped item: %+v", got)
	}

	if err := repo.MarkFailed(ctx, item.ID, "endpoint returned 500", intPtr(500)); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != domain.ACPOutboxStatusFailed || got.ErrorMessage == "" || got.LastStatusCode == nil || *got.LastStatusCode != 500 || got.DispatchAttempts != 1 || got.LastAttemptedAt == nil {
		t.Fatalf("unexpected failed item: %+v", got)
	}

	if err := repo.MarkDispatched(ctx, item.ID, 202); err != nil {
		t.Fatalf("MarkDispatched: %v", err)
	}
	got, _ = repo.Get(ctx, item.ID)
	if got.Status != domain.ACPOutboxStatusDispatched || got.LastStatusCode == nil || *got.LastStatusCode != 202 || got.DispatchAttempts != 2 || got.DispatchedAt == nil {
		t.Fatalf("unexpected dispatched item: %+v", got)
	}
}

func TestACPOutboxRepositoryAllowsMalformedOutputWithoutEventJSON(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	repo := NewACPOutboxRepository(db)

	item := domain.NewACPOutboxItem("msg_bad", nil, "not-json")
	item.Status = domain.ACPOutboxStatusFailed
	item.ErrorMessage = "parse error"
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Event != nil || got.EventJSON != "" || got.RawLLMOutput != "not-json" || got.Status != domain.ACPOutboxStatusFailed {
		t.Fatalf("unexpected malformed item: %+v", got)
	}

	var eventJSON sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT event_json FROM acp_outbox WHERE id = ?`, item.ID).Scan(&eventJSON); err != nil {
		t.Fatalf("query event_json: %v", err)
	}
	if eventJSON.Valid {
		t.Fatalf("event_json should be NULL, got %q", eventJSON.String)
	}
}

func intPtr(v int) *int {
	return &v
}

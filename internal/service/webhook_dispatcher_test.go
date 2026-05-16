package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/security"
)

func TestWebhookDispatcherPostsSignedEvent(t *testing.T) {
	var gotSignature string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSignature = r.Header.Get(security.SignatureHeader)
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	store := &memoryWebhookRepo{hooks: []domain.WebhookConfig{
		*domain.NewWebhookConfig(server.URL, []string{"inbound.telegram"}),
	}}
	store.hooks[0].Secret = "secret"
	dispatcher := NewWebhookDispatcher(WebhookDispatcherDeps{Webhooks: store, HTTPClient: server.Client()})

	msg := domain.NewMessage(domain.DirectionInbound, "hello", "telegram")
	if err := dispatcher.Dispatch(context.Background(), "inbound.telegram", *msg); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !security.VerifyHMACSHA256("secret", gotBody, gotSignature) {
		t.Fatalf("invalid signature %q for %s", gotSignature, string(gotBody))
	}
	if !store.triggered[store.hooks[0].ID] {
		t.Fatalf("hook should be marked triggered")
	}
}

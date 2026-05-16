package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func TestSendPostsBotAPISendMessage(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":123}}`))
	}))
	defer server.Close()

	adapter := New(AdapterConfig{BaseURL: server.URL, HTTPClient: server.Client()})
	msg := domain.NewMessage(domain.DirectionOutbound, "hello", "api")
	channel := domain.Channel{
		Platform: domain.PlatformTelegram,
		Config: map[string]any{
			"bot_token": "token",
			"chat_id":   "42",
		},
	}

	result, err := adapter.Send(context.Background(), *msg, channel)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/bottoken/sendMessage" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["chat_id"] != "42" || gotBody["text"] != "hello" {
		t.Fatalf("body = %+v", gotBody)
	}
	if result.MessageID != "123" {
		t.Fatalf("message id = %q", result.MessageID)
	}
}

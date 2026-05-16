package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func TestSendPostsChannelMessage(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"id":"abc"}`))
	}))
	defer server.Close()

	adapter := New(AdapterConfig{RESTBaseURL: server.URL, HTTPClient: server.Client()})
	msg := domain.NewMessage(domain.DirectionOutbound, "hello", "api")
	channel := domain.Channel{
		Platform: domain.PlatformDiscord,
		Config: map[string]any{
			"bot_token":  "token",
			"channel_id": "42",
		},
	}

	result, err := adapter.Send(context.Background(), *msg, channel)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotPath != "/channels/42/messages" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bot token" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotBody["content"] != "hello" {
		t.Fatalf("body = %+v", gotBody)
	}
	if result.MessageID != "abc" {
		t.Fatalf("message id = %q", result.MessageID)
	}
}

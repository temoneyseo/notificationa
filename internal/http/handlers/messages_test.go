package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/storage/sqlite"
)

func TestNotifyDefaultsToDiscord(t *testing.T) {
	env := newHandlerTestEnv(t)

	resp := performJSON(env.router, http.MethodPost, "/api/v1/notify", map[string]any{
		"text": "hello agent",
	})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	channels := body["channels"].([]any)
	if len(channels) != 1 || channels[0] != "discord" {
		t.Fatalf("channels = %+v", channels)
	}
	if body["content_original"] != "hello agent" {
		t.Fatalf("body = %+v", body)
	}
}

func TestNotifyAllSendsAllActiveChannels(t *testing.T) {
	env := newHandlerTestEnv(t)

	resp := performJSON(env.router, http.MethodPost, "/api/v1/notify", map[string]any{
		"text": "hello all",
		"to":   "all",
	})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	channels := body["channels"].([]any)
	if len(channels) != 2 {
		t.Fatalf("channels = %+v", channels)
	}
}

func TestNotifyAcceptsStringAndArrayTo(t *testing.T) {
	env := newHandlerTestEnv(t)

	resp := performJSON(env.router, http.MethodPost, "/api/v1/notify", map[string]any{
		"text": "hello telegram",
		"to":   "telegram",
	})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	resp = performJSON(env.router, http.MethodPost, "/api/v1/notify", map[string]any{
		"text": "hello both",
		"to":   []string{"telegram", "discord"},
	})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestNotifyRequiresText(t *testing.T) {
	env := newHandlerTestEnv(t)

	resp := performJSON(env.router, http.MethodPost, "/api/v1/notify", map[string]any{})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}
func TestPostMessagesSendsAndStoresMessage(t *testing.T) {
	env := newHandlerTestEnv(t)
	resp := performJSON(env.router, http.MethodPost, "/api/v1/messages", map[string]any{
		"content":       "hello",
		"channels":      []string{"telegram"},
		"priority":      "high",
		"ai_processing": "none",
		"metadata": map[string]any{
			"source": "test",
		},
	})
	if resp.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] == "" || body["status"] != string(domain.StatusSent) {
		t.Fatalf("body = %+v", body)
	}
}

func TestInboxListsInboundMessages(t *testing.T) {
	env := newHandlerTestEnv(t)
	messages := sqlite.NewMessageRepository(env.db)
	msg := domain.NewMessage(domain.DirectionInbound, "hello", "telegram")
	msg.Status = domain.StatusSent
	if err := messages.Create(t.Context(), msg); err != nil {
		t.Fatal(err)
	}

	resp := performJSON(env.router, http.MethodGet, "/api/v1/messages/inbox?channel=telegram&limit=10", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Data []domain.Message `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || body.Data[0].ID != msg.ID {
		t.Fatalf("body = %+v", body)
	}
}

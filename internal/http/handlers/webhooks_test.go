package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/user/notification-hub/internal/storage/sqlite"
)

func TestWebhookCRUDEncryptsSecret(t *testing.T) {
	env := newHandlerTestEnv(t)
	resp := performJSON(env.router, http.MethodPost, "/api/v1/webhooks", map[string]any{
		"url":    "https://example.test/hook",
		"events": []string{"inbound.telegram"},
		"secret": "whsec",
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created["secret"] != "********" {
		t.Fatalf("created = %+v", created)
	}
	repo := sqlite.NewWebhookRepository(env.db)
	stored, err := repo.Get(t.Context(), created["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Secret == "whsec" {
		t.Fatalf("secret should be encrypted")
	}
}

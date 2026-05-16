package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/adapters"
	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/security"
	"github.com/user/notification-hub/internal/service"
	"github.com/user/notification-hub/internal/storage/sqlite"
)

type handlerTestEnv struct {
	router   *gin.Engine
	db       *sqlite.DB
	cipher   *security.Cipher
	registry *service.AdapterRegistry
}

func newHandlerTestEnv(t *testing.T) *handlerTestEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	cipher, err := security.NewCipher("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	messages := sqlite.NewMessageRepository(db)
	channels := service.NewConfigChannelStore([]domain.Channel{
		{
			ID:        "telegram-main",
			Platform:  domain.PlatformTelegram,
			Name:      "telegram-main",
			Config:    map[string]any{"bot_token": "telegram-token", "chat_id": "-100"},
			IsActive:  true,
			IsDefault: true,
		},
		{
			ID:       "discord-main",
			Platform: domain.PlatformDiscord,
			Name:     "discord-main",
			Config:   map[string]any{"bot_token": "discord-token", "channel_id": "123"},
			IsActive: true,
		},
	})
	webhooks := sqlite.NewWebhookRepository(db)
	registry := service.NewAdapterRegistry()
	registry.Register(&captureAdapter{platform: "telegram"})
	registry.Register(&captureAdapter{platform: "discord"})
	pipeline := service.NewPipeline(service.PipelineDeps{
		Messages: messages,
		Channels: channels,
		Adapters: registry,
		AI:       ai.SafeProcessor{Processor: ai.NoopProcessor{}},
		Cipher:   cipher,
	})
	router := gin.New()
	RegisterRoutes(router, Dependencies{
		Messages: messages,
		Channels: channels,
		Webhooks: webhooks,
		Pipeline: pipeline,
		Cipher:   cipher,
	})
	return &handlerTestEnv{router: router, db: db, cipher: cipher, registry: registry}
}

func performJSON(router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	return resp
}

type captureAdapter struct {
	platform string
	sent     []domain.Message
}

func (a *captureAdapter) Platform() string { return a.platform }

func (a *captureAdapter) Send(_ context.Context, msg domain.Message, _ domain.Channel) (adapters.SendResult, error) {
	a.sent = append(a.sent, msg)
	return adapters.SendResult{MessageID: a.platform + "-id"}, nil
}

func (a *captureAdapter) StartListening(context.Context) error { return nil }

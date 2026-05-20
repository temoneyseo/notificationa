package discord

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/user/notification-hub/internal/adapters"
)

type noopInboundHandler struct{}

func (noopInboundHandler) HandleInbound(context.Context, adapters.InboundMessage) error { return nil }

func TestGatewayStopsOnAuthenticationClose(t *testing.T) {
	upgrader := websocket.Upgrader{}
	connections := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections++
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		_ = conn.WriteJSON(gatewayPayload{Op: gatewayHello, Data: mustJSON(map[string]any{"heartbeat_interval": 1000})})
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4004, "authentication failed"), time.Now().Add(time.Second))
		_ = conn.Close()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gateway := NewGateway("ws"+strings.TrimPrefix(server.URL, "http"), "bad-token", noopInboundHandler{})

	err := gateway.Start(ctx)
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if !errors.Is(err, ErrGatewayAuthenticationFailed) {
		t.Fatalf("expected ErrGatewayAuthenticationFailed, got %v", err)
	}
	if connections != 1 {
		t.Fatalf("connections = %d, want 1", connections)
	}
}

func TestGatewayReconnectStormBacksOff(t *testing.T) {
	upgrader := websocket.Upgrader{}
	connections := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connections++
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseAbnormalClosure, "temporary failure"), time.Now().Add(time.Second))
		_ = conn.Close()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gateway := NewGateway("ws"+strings.TrimPrefix(server.URL, "http"), "token", noopInboundHandler{})
	gateway.retryDelay = time.Millisecond
	gateway.stormBackoff = time.Second

	err := gateway.Start(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline after backoff, got %v", err)
	}
	if connections != 5 {
		t.Fatalf("connections = %d, want 5 before backoff", connections)
	}
}

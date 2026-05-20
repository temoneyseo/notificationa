package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestGatewayHeartbeatSendsNullBeforeSequence(t *testing.T) {
	handlerErr := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 10); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayIdentify {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("identify op = %d, want %d", payload.Op, gatewayIdentify))
			return
		}
		payload, raw, err := readGatewayPayload(conn)
		if err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload.Op != gatewayHeartbeat {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("heartbeat op = %d, want %d", payload.Op, gatewayHeartbeat))
			return
		}
		data, ok := raw["d"]
		if !ok {
			recordGatewayHandlerError(handlerErr, errors.New("heartbeat payload missing d field"))
			return
		}
		if string(data) != "null" {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("heartbeat d = %s, want null", data))
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gateway := NewGateway(gatewayTestURL(server), "token", noopInboundHandler{})

	_ = gateway.connectOnce(ctx)
	assertNoGatewayHandlerError(t, handlerErr)
}

func TestGatewayHeartbeatUsesLatestDispatchSequence(t *testing.T) {
	handlerErr := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 10); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayIdentify {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("identify op = %d, want %d", payload.Op, gatewayIdentify))
			return
		}
		if err := conn.WriteJSON(map[string]any{
			"op": gatewayDispatch,
			"s":  42,
			"t":  "READY",
			"d":  map[string]any{},
		}); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		payload, raw, err := readGatewayPayload(conn)
		if err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload.Op != gatewayHeartbeat {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("heartbeat op = %d, want %d", payload.Op, gatewayHeartbeat))
			return
		}
		if string(raw["d"]) != "42" {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("heartbeat d = %s, want 42", raw["d"]))
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gateway := NewGateway(gatewayTestURL(server), "token", noopInboundHandler{})

	_ = gateway.connectOnce(ctx)
	assertNoGatewayHandlerError(t, handlerErr)
}

func TestGatewayRespondsToServerHeartbeatRequest(t *testing.T) {
	handlerErr := make(chan error, 1)
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 60_000); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayIdentify {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("identify op = %d, want %d", payload.Op, gatewayIdentify))
			return
		}
		if err := conn.WriteJSON(gatewayPayload{Op: gatewayHeartbeat}); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		payload, raw, err := readGatewayPayload(conn)
		if err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload.Op != gatewayHeartbeat {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("heartbeat op = %d, want %d", payload.Op, gatewayHeartbeat))
			return
		}
		if string(raw["d"]) != "null" {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("heartbeat d = %s, want null", raw["d"]))
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gateway := NewGateway(gatewayTestURL(server), "token", noopInboundHandler{})

	_ = gateway.connectOnce(ctx)
	assertNoGatewayHandlerError(t, handlerErr)
}

func TestGatewayHeartbeatACKAllowsNextHeartbeat(t *testing.T) {
	handlerErr := make(chan error, 1)
	secondHeartbeat := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 10); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayIdentify {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("identify op = %d, want %d", payload.Op, gatewayIdentify))
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayHeartbeat {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("first heartbeat op = %d, want %d", payload.Op, gatewayHeartbeat))
			return
		}
		if err := conn.WriteJSON(gatewayPayload{Op: gatewayHeartbeatACK}); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayHeartbeat {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("second heartbeat op = %d, want %d", payload.Op, gatewayHeartbeat))
			return
		}
		closeOnce(secondHeartbeat)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	gateway := NewGateway(gatewayTestURL(server), "token", noopInboundHandler{})

	startErr := make(chan error, 1)
	go func() {
		startErr <- gateway.connectOnce(ctx)
	}()

	select {
	case <-secondHeartbeat:
	case err := <-handlerErr:
		t.Fatal(err)
	case err := <-startErr:
		t.Fatalf("connectOnce returned before second heartbeat: %v", err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for second heartbeat: %v", ctx.Err())
	}
	cancel()
	<-startErr
	assertNoGatewayHandlerError(t, handlerErr)
}

func TestGatewayMissingHeartbeatACKReconnects(t *testing.T) {
	handlerErr := make(chan error, 1)
	reconnected := make(chan struct{})
	upgrader := websocket.Upgrader{}
	var connections int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddInt32(&connections, 1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if id == 2 {
			closeOnce(reconnected)
			return
		}
		if err := writeGatewayHello(conn, 10); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayIdentify {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("identify op = %d, want %d", payload.Op, gatewayIdentify))
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayHeartbeat {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("heartbeat op = %d, want %d", payload.Op, gatewayHeartbeat))
			return
		}
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gateway := NewGateway(gatewayTestURL(server), "token", noopInboundHandler{})
	gateway.retryDelay = time.Millisecond
	startErr := make(chan error, 1)
	go func() {
		startErr <- gateway.Start(ctx)
	}()

	select {
	case <-reconnected:
	case err := <-handlerErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for reconnect: %v", ctx.Err())
	}
	cancel()
	<-startErr
	assertNoGatewayHandlerError(t, handlerErr)
	if got := atomic.LoadInt32(&connections); got < 2 {
		t.Fatalf("connections = %d, want at least 2", got)
	}
}

func TestGatewayReconnectOpcodeReconnects(t *testing.T) {
	handlerErr := make(chan error, 1)
	reconnected := make(chan struct{})
	upgrader := websocket.Upgrader{}
	var connections int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddInt32(&connections, 1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if id == 2 {
			closeOnce(reconnected)
			return
		}
		if err := writeGatewayHello(conn, 60_000); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayIdentify {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("identify op = %d, want %d", payload.Op, gatewayIdentify))
			return
		}
		if err := conn.WriteJSON(gatewayPayload{Op: gatewayReconnect}); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gateway := NewGateway(gatewayTestURL(server), "token", noopInboundHandler{})
	gateway.retryDelay = time.Millisecond
	startErr := make(chan error, 1)
	go func() {
		startErr <- gateway.Start(ctx)
	}()

	select {
	case <-reconnected:
	case err := <-handlerErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for reconnect: %v", ctx.Err())
	}
	cancel()
	<-startErr
	assertNoGatewayHandlerError(t, handlerErr)
	if got := atomic.LoadInt32(&connections); got < 2 {
		t.Fatalf("connections = %d, want at least 2", got)
	}
}

func TestGatewayReadyStateEnablesResumeOnReconnect(t *testing.T) {
	handlerErr := make(chan error, 1)
	resumed := make(chan struct{})
	upgrader := websocket.Upgrader{}
	resumeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("resume upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 60_000); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		payload, raw, err := readGatewayPayload(conn)
		if err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload.Op != gatewayResume {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("resume op = %d, want %d", payload.Op, gatewayResume))
			return
		}
		var data struct {
			Token     string `json:"token"`
			SessionID string `json:"session_id"`
			Seq       int64  `json:"seq"`
		}
		if err := json.Unmarshal(raw["d"], &data); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if data.Token != "token" || data.SessionID != "session-1" || data.Seq != 0 {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("resume data = %+v", data))
			return
		}
		closeOnce(resumed)
	}))
	defer resumeServer.Close()

	originalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("original upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 60_000); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if payload, _, err := readGatewayPayload(conn); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		} else if payload.Op != gatewayIdentify {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("identify op = %d, want %d", payload.Op, gatewayIdentify))
			return
		}
		if err := conn.WriteJSON(map[string]any{
			"op": gatewayDispatch,
			"s":  0,
			"t":  "READY",
			"d": map[string]any{
				"session_id":         "session-1",
				"resume_gateway_url": gatewayTestURL(resumeServer),
			},
		}); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		if err := conn.WriteJSON(gatewayPayload{Op: gatewayReconnect}); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
	}))
	defer originalServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gateway := NewGateway(gatewayTestURL(originalServer), "token", noopInboundHandler{})
	gateway.retryDelay = time.Millisecond
	startErr := make(chan error, 1)
	go func() {
		startErr <- gateway.Start(ctx)
	}()

	select {
	case <-resumed:
	case err := <-handlerErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for resume: %v", ctx.Err())
	}
	cancel()
	<-startErr
	assertNoGatewayHandlerError(t, handlerErr)
}

func TestGatewayInvalidSessionFalseClearsResumeState(t *testing.T) {
	handlerErr := make(chan error, 1)
	identifiedAfterInvalidSession := make(chan struct{})
	upgrader := websocket.Upgrader{}
	var connections int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddInt32(&connections, 1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 60_000); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		payload, _, err := readGatewayPayload(conn)
		if err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		switch id {
		case 1:
			if payload.Op != gatewayIdentify {
				recordGatewayHandlerError(handlerErr, fmt.Errorf("first op = %d, want identify", payload.Op))
				return
			}
			if err := conn.WriteJSON(map[string]any{
				"op": gatewayDispatch,
				"s":  7,
				"t":  "READY",
				"d": map[string]any{
					"session_id":         "session-1",
					"resume_gateway_url": serverURL,
				},
			}); err != nil {
				recordGatewayHandlerError(handlerErr, err)
				return
			}
			if err := conn.WriteJSON(gatewayPayload{Op: gatewayReconnect}); err != nil {
				recordGatewayHandlerError(handlerErr, err)
				return
			}
		case 2:
			if payload.Op != gatewayResume {
				recordGatewayHandlerError(handlerErr, fmt.Errorf("second op = %d, want resume", payload.Op))
				return
			}
			if err := conn.WriteJSON(gatewayPayload{Op: gatewayInvalidSession, Data: mustJSON(false)}); err != nil {
				recordGatewayHandlerError(handlerErr, err)
				return
			}
		case 3:
			if payload.Op != gatewayIdentify {
				recordGatewayHandlerError(handlerErr, fmt.Errorf("third op = %d, want identify after invalid session false", payload.Op))
				return
			}
			closeOnce(identifiedAfterInvalidSession)
		default:
			recordGatewayHandlerError(handlerErr, fmt.Errorf("unexpected connection %d", id))
		}
	}))
	defer server.Close()
	serverURL = gatewayTestURL(server)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gateway := NewGateway(serverURL, "token", noopInboundHandler{})
	gateway.retryDelay = time.Millisecond
	startErr := make(chan error, 1)
	go func() {
		startErr <- gateway.Start(ctx)
	}()

	select {
	case <-identifiedAfterInvalidSession:
	case err := <-handlerErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for identify after invalid session false: %v", ctx.Err())
	}
	cancel()
	<-startErr
	assertNoGatewayHandlerError(t, handlerErr)
}

func TestGatewayInvalidSessionTrueKeepsResumeState(t *testing.T) {
	handlerErr := make(chan error, 1)
	resumedAfterInvalidSession := make(chan struct{})
	upgrader := websocket.Upgrader{}
	var connections int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := atomic.AddInt32(&connections, 1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			recordGatewayHandlerError(handlerErr, fmt.Errorf("upgrade: %w", err))
			return
		}
		defer conn.Close()

		if err := writeGatewayHello(conn, 60_000); err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		payload, raw, err := readGatewayPayload(conn)
		if err != nil {
			recordGatewayHandlerError(handlerErr, err)
			return
		}
		switch id {
		case 1:
			if payload.Op != gatewayIdentify {
				recordGatewayHandlerError(handlerErr, fmt.Errorf("first op = %d, want identify", payload.Op))
				return
			}
			if err := conn.WriteJSON(map[string]any{
				"op": gatewayDispatch,
				"s":  9,
				"t":  "READY",
				"d": map[string]any{
					"session_id":         "session-1",
					"resume_gateway_url": serverURL,
				},
			}); err != nil {
				recordGatewayHandlerError(handlerErr, err)
				return
			}
			if err := conn.WriteJSON(gatewayPayload{Op: gatewayReconnect}); err != nil {
				recordGatewayHandlerError(handlerErr, err)
				return
			}
		case 2:
			if payload.Op != gatewayResume {
				recordGatewayHandlerError(handlerErr, fmt.Errorf("second op = %d, want resume", payload.Op))
				return
			}
			if err := conn.WriteJSON(gatewayPayload{Op: gatewayInvalidSession, Data: mustJSON(true)}); err != nil {
				recordGatewayHandlerError(handlerErr, err)
				return
			}
		case 3:
			if payload.Op != gatewayResume {
				recordGatewayHandlerError(handlerErr, fmt.Errorf("third op = %d, want resume after invalid session true", payload.Op))
				return
			}
			var data struct {
				SessionID string `json:"session_id"`
				Seq       int64  `json:"seq"`
			}
			if err := json.Unmarshal(raw["d"], &data); err != nil {
				recordGatewayHandlerError(handlerErr, err)
				return
			}
			if data.SessionID != "session-1" || data.Seq != 9 {
				recordGatewayHandlerError(handlerErr, fmt.Errorf("resume data = %+v", data))
				return
			}
			closeOnce(resumedAfterInvalidSession)
		default:
			recordGatewayHandlerError(handlerErr, fmt.Errorf("unexpected connection %d", id))
		}
	}))
	defer server.Close()
	serverURL = gatewayTestURL(server)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gateway := NewGateway(serverURL, "token", noopInboundHandler{})
	gateway.retryDelay = time.Millisecond
	startErr := make(chan error, 1)
	go func() {
		startErr <- gateway.Start(ctx)
	}()

	select {
	case <-resumedAfterInvalidSession:
	case err := <-handlerErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for resume after invalid session true: %v", ctx.Err())
	}
	cancel()
	<-startErr
	assertNoGatewayHandlerError(t, handlerErr)
}

func gatewayTestURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func writeGatewayHello(conn *websocket.Conn, intervalMillis int) error {
	return conn.WriteJSON(gatewayPayload{Op: gatewayHello, Data: mustJSON(map[string]any{"heartbeat_interval": intervalMillis})})
}

func readGatewayPayload(conn *websocket.Conn) (gatewayPayload, map[string]json.RawMessage, error) {
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return gatewayPayload{}, nil, err
	}
	var payload gatewayPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return gatewayPayload{}, nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return gatewayPayload{}, nil, err
	}
	return payload, raw, nil
}

func recordGatewayHandlerError(ch chan<- error, err error) {
	if err == nil {
		return
	}
	select {
	case ch <- err:
	default:
	}
}

func assertNoGatewayHandlerError(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case err := <-ch:
		t.Fatal(err)
	default:
	}
}

func closeOnce(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

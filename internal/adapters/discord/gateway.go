package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/user/notification-hub/internal/adapters"
)

const (
	gatewayDispatch       = 0
	gatewayHeartbeat      = 1
	gatewayIdentify       = 2
	gatewayResume         = 6
	gatewayReconnect      = 7
	gatewayInvalidSession = 9
	gatewayHello          = 10
	gatewayHeartbeatACK   = 11
)

var (
	ErrGatewayAuthenticationFailed = errors.New("discord gateway authentication failed")
	errGatewayReconnectable        = errors.New("discord gateway reconnectable")
)

type Gateway struct {
	url          string
	token        string
	handler      adapters.InboundHandler
	dialer       *websocket.Dialer
	retryDelay   time.Duration
	stormBackoff time.Duration

	stateMu          sync.RWMutex
	sequence         int64
	hasSequence      bool
	sessionID        string
	resumeGatewayURL string
}

func NewGateway(url, token string, handler adapters.InboundHandler) *Gateway {
	if url == "" {
		url = "wss://gateway.discord.gg/?v=10&encoding=json"
	}
	return &Gateway{
		url:          url,
		token:        token,
		handler:      handler,
		dialer:       websocket.DefaultDialer,
		retryDelay:   5 * time.Second,
		stormBackoff: 5 * time.Minute,
	}
}

func (g *Gateway) Start(ctx context.Context) error {
	if g.token == "" || g.handler == nil {
		return nil
	}
	attempts := 0
	for {
		attempts++
		if err := g.connectOnce(ctx); err != nil {
			if errors.Is(err, ErrGatewayAuthenticationFailed) {
				log.Printf("ERROR discord gateway authentication failed: token is invalid or reset by Discord. Stop reconnecting. Generate a new DISCORD_BOT_TOKEN before restarting.")
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("WARN discord gateway disconnected: attempt=%d error=%v", attempts, err)
			if attempts >= 5 {
				log.Printf("WARN discord gateway reconnect storm: %d attempts, backing off for %s", attempts, g.stormBackoff)
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(g.stormBackoff):
				}
				attempts = 0
				continue
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(g.retryDelay):
			}
			continue
		}
		return nil
	}
}

func (g *Gateway) connectOnce(ctx context.Context) error {
	connectURL, resume := g.nextConnection()
	conn, _, err := g.dialer.DialContext(ctx, connectURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	writer := &gatewayConnWriter{conn: conn}
	heartbeatState := &gatewayHeartbeatState{}
	heartbeatState.acked.Store(true)
	heartbeatErr := make(chan error, 1)
	heartbeatStarted := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-heartbeatErr:
			return err
		default:
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case heartbeatErr := <-heartbeatErr:
				return heartbeatErr
			default:
			}
			return classifyGatewayReadError(err)
		}
		var event gatewayPayload
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		switch event.Op {
		case gatewayHello:
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"`
			}
			_ = json.Unmarshal(event.Data, &hello)
			if err := writer.writeJSON(g.identifyPayload(resume)); err != nil {
				return err
			}
			if !heartbeatStarted {
				heartbeatStarted = true
				go g.heartbeat(connCtx, writer, heartbeatState, time.Duration(hello.HeartbeatInterval)*time.Millisecond, heartbeatErr)
			}
		case gatewayDispatch:
			g.updateSequence(event.Sequence)
			if event.Type == "READY" {
				if err := g.handleReady(event.Data); err != nil {
					return err
				}
			}
			if event.Type == "MESSAGE_CREATE" {
				if err := g.handleMessage(ctx, event.Data); err != nil {
					return err
				}
			}
		case gatewayHeartbeat:
			if err := g.sendHeartbeat(writer, heartbeatState, false); err != nil {
				return err
			}
		case gatewayReconnect:
			return fmt.Errorf("%w: discord requested reconnect", errGatewayReconnectable)
		case gatewayInvalidSession:
			var resumable bool
			_ = json.Unmarshal(event.Data, &resumable)
			if !resumable {
				g.clearResumeState()
			}
			return fmt.Errorf("%w: discord invalid session resumable=%t", errGatewayReconnectable, resumable)
		case gatewayHeartbeatACK:
			heartbeatState.acked.Store(true)
		}
	}
}

func classifyGatewayReadError(err error) error {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) && closeErr.Code == 4004 {
		return fmt.Errorf("%w: discord close code %d %s", ErrGatewayAuthenticationFailed, closeErr.Code, closeErr.Text)
	}
	return err
}

func (g *Gateway) nextConnection() (string, bool) {
	g.stateMu.RLock()
	defer g.stateMu.RUnlock()
	if g.sessionID != "" && g.resumeGatewayURL != "" && g.hasSequence {
		return g.resumeGatewayURL, true
	}
	return g.url, false
}

func (g *Gateway) identifyPayload(resume bool) gatewayPayload {
	if !resume {
		return gatewayPayload{Op: gatewayIdentify, Data: mustJSON(map[string]any{
			"token":   g.token,
			"intents": 33280,
			"properties": map[string]string{
				"os":      "linux",
				"browser": "notification-hub",
				"device":  "notification-hub",
			},
		})}
	}
	g.stateMu.RLock()
	sessionID := g.sessionID
	sequence := g.sequence
	g.stateMu.RUnlock()
	return gatewayPayload{Op: gatewayResume, Data: mustJSON(map[string]any{
		"token":      g.token,
		"session_id": sessionID,
		"seq":        sequence,
	})}
}

func (g *Gateway) updateSequence(data json.RawMessage) {
	if len(data) == 0 || string(data) == "null" {
		return
	}
	var sequence int64
	if err := json.Unmarshal(data, &sequence); err != nil {
		return
	}
	g.stateMu.Lock()
	g.sequence = sequence
	g.hasSequence = true
	g.stateMu.Unlock()
}

func (g *Gateway) handleReady(data json.RawMessage) error {
	var ready struct {
		SessionID        string `json:"session_id"`
		ResumeGatewayURL string `json:"resume_gateway_url"`
	}
	if err := json.Unmarshal(data, &ready); err != nil {
		return err
	}
	g.stateMu.Lock()
	g.sessionID = ready.SessionID
	g.resumeGatewayURL = ready.ResumeGatewayURL
	g.stateMu.Unlock()
	return nil
}

func (g *Gateway) clearResumeState() {
	g.stateMu.Lock()
	g.sequence = 0
	g.hasSequence = false
	g.sessionID = ""
	g.resumeGatewayURL = ""
	g.stateMu.Unlock()
}

func (g *Gateway) heartbeatData() json.RawMessage {
	g.stateMu.RLock()
	defer g.stateMu.RUnlock()
	if !g.hasSequence {
		return json.RawMessage("null")
	}
	return mustJSON(g.sequence)
}

func (g *Gateway) sendHeartbeat(writer *gatewayConnWriter, state *gatewayHeartbeatState, requirePreviousACK bool) error {
	if requirePreviousACK && !state.acked.Load() {
		_ = writer.close()
		return fmt.Errorf("%w: discord heartbeat ack timeout", errGatewayReconnectable)
	}
	if err := writer.writeJSON(gatewayPayload{Op: gatewayHeartbeat, Data: g.heartbeatData()}); err != nil {
		return err
	}
	state.acked.Store(false)
	return nil
}

func (g *Gateway) heartbeat(ctx context.Context, writer *gatewayConnWriter, state *gatewayHeartbeatState, interval time.Duration, errCh chan<- error) {
	if interval <= 0 {
		interval = 45 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := g.sendHeartbeat(writer, state, true); err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}
}

type gatewayConnWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *gatewayConnWriter) writeJSON(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteJSON(v)
}

func (w *gatewayConnWriter) close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.Close()
}

type gatewayHeartbeatState struct {
	acked atomic.Bool
}

func (g *Gateway) handleMessage(ctx context.Context, data json.RawMessage) error {
	var message struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
		Content   string `json:"content"`
		Author    struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Bot      bool   `json:"bot"`
		} `json:"author"`
	}
	if err := json.Unmarshal(data, &message); err != nil {
		return err
	}
	if message.Author.Bot || message.Content == "" {
		return nil
	}
	return g.handler.HandleInbound(ctx, adapters.InboundMessage{
		Platform:          "discord",
		ChannelID:         message.ChannelID,
		PlatformMessageID: message.ID,
		AuthorID:          message.Author.ID,
		AuthorName:        message.Author.Username,
		Content:           message.Content,
		Metadata: map[string]any{
			"channel_id": message.ChannelID,
		},
	})
}

type gatewayPayload struct {
	Op       int             `json:"op"`
	Data     json.RawMessage `json:"d"`
	Sequence json.RawMessage `json:"s,omitempty"`
	Type     string          `json:"t,omitempty"`
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("marshal gateway payload: %w", err))
	}
	return b
}

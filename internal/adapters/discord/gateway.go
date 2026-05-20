package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
	"github.com/user/notification-hub/internal/adapters"
)

const (
	gatewayDispatch  = 0
	gatewayHeartbeat = 1
	gatewayIdentify  = 2
	gatewayHello     = 10
)

var ErrGatewayAuthenticationFailed = errors.New("discord gateway authentication failed")

type Gateway struct {
	url          string
	token        string
	handler      adapters.InboundHandler
	dialer       *websocket.Dialer
	retryDelay   time.Duration
	stormBackoff time.Duration
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
	conn, _, err := g.dialer.DialContext(ctx, g.url, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
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
			go heartbeat(connCtx, conn, time.Duration(hello.HeartbeatInterval)*time.Millisecond)
			if err := conn.WriteJSON(gatewayPayload{Op: gatewayIdentify, Data: mustJSON(map[string]any{
				"token":   g.token,
				"intents": 33280,
				"properties": map[string]string{
					"os":      "linux",
					"browser": "notification-hub",
					"device":  "notification-hub",
				},
			})}); err != nil {
				return err
			}
		case gatewayDispatch:
			if event.Type == "MESSAGE_CREATE" {
				if err := g.handleMessage(ctx, event.Data); err != nil {
					return err
				}
			}
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

func heartbeat(ctx context.Context, conn *websocket.Conn, interval time.Duration) {
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
			_ = conn.WriteJSON(gatewayPayload{Op: gatewayHeartbeat})
		}
	}
}

type gatewayPayload struct {
	Op   int             `json:"op"`
	Data json.RawMessage `json:"d,omitempty"`
	Type string          `json:"t,omitempty"`
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("marshal gateway payload: %w", err))
	}
	return b
}

package discord

import (
	"context"
	"encoding/json"
	"fmt"
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

type Gateway struct {
	url     string
	token   string
	handler adapters.InboundHandler
	dialer  *websocket.Dialer
}

func NewGateway(url, token string, handler adapters.InboundHandler) *Gateway {
	if url == "" {
		url = "wss://gateway.discord.gg/?v=10&encoding=json"
	}
	return &Gateway{url: url, token: token, handler: handler, dialer: websocket.DefaultDialer}
}

func (g *Gateway) Start(ctx context.Context) error {
	if g.token == "" || g.handler == nil {
		return nil
	}
	for {
		if err := g.connectOnce(ctx); err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
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
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
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
			go heartbeat(ctx, conn, time.Duration(hello.HeartbeatInterval)*time.Millisecond)
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

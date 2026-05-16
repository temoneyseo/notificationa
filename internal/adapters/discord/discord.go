package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/adapters"
	"github.com/user/notification-hub/internal/domain"
)

type AdapterConfig struct {
	RESTBaseURL string
	GatewayURL  string
	HTTPClient  *http.Client
}

type Adapter struct {
	restBaseURL string
	gatewayURL  string
	client      *http.Client
}

func New(cfg AdapterConfig) *Adapter {
	if cfg.RESTBaseURL == "" {
		cfg.RESTBaseURL = "https://discord.com/api/v10"
	}
	if cfg.GatewayURL == "" {
		cfg.GatewayURL = "wss://gateway.discord.gg/?v=10&encoding=json"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Adapter{
		restBaseURL: strings.TrimRight(cfg.RESTBaseURL, "/"),
		gatewayURL:  cfg.GatewayURL,
		client:      cfg.HTTPClient,
	}
}

func (a *Adapter) Platform() string {
	return string(domain.PlatformDiscord)
}

func (a *Adapter) Send(ctx context.Context, msg domain.Message, channel domain.Channel) (adapters.SendResult, error) {
	token, _ := channel.Config["bot_token"].(string)
	channelID, _ := channel.Config["channel_id"].(string)
	if token == "" || channelID == "" {
		return adapters.SendResult{}, errors.New("discord bot_token and channel_id are required")
	}
	content := msg.ContentProcessed
	if content == "" {
		content = msg.ContentOriginal
	}
	body, err := json.Marshal(map[string]any{"content": content})
	if err != nil {
		return adapters.SendResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/channels/%s/messages", a.restBaseURL, channelID), bytes.NewReader(body))
	if err != nil {
		return adapters.SendResult{}, err
	}
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return adapters.SendResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return adapters.SendResult{}, fmt.Errorf("discord returned status %d", resp.StatusCode)
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return adapters.SendResult{}, err
	}
	return adapters.SendResult{MessageID: parsed.ID}, nil
}

func (a *Adapter) StartListening(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

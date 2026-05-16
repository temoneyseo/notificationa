package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/adapters"
	"github.com/user/notification-hub/internal/domain"
)

type AdapterConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Adapter struct {
	baseURL string
	client  *http.Client
}

func New(cfg AdapterConfig) *Adapter {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.telegram.org"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Adapter{baseURL: strings.TrimRight(cfg.BaseURL, "/"), client: cfg.HTTPClient}
}

func (a *Adapter) Platform() string {
	return string(domain.PlatformTelegram)
}

func (a *Adapter) Send(ctx context.Context, msg domain.Message, channel domain.Channel) (adapters.SendResult, error) {
	token, _ := channel.Config["bot_token"].(string)
	chatID, _ := channel.Config["chat_id"].(string)
	if token == "" || chatID == "" {
		return adapters.SendResult{}, errors.New("telegram bot_token and chat_id are required")
	}
	payload := map[string]any{
		"chat_id": chatID,
		"text":    msg.ContentProcessed,
	}
	if payload["text"] == "" {
		payload["text"] = msg.ContentOriginal
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return adapters.SendResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/sendMessage", a.baseURL, token), bytes.NewReader(body))
	if err != nil {
		return adapters.SendResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return adapters.SendResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return adapters.SendResult{}, fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}
	var parsed struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return adapters.SendResult{}, err
	}
	if !parsed.OK {
		if parsed.Description == "" {
			parsed.Description = "telegram send failed"
		}
		return adapters.SendResult{}, errors.New(parsed.Description)
	}
	return adapters.SendResult{MessageID: strconv.FormatInt(parsed.Result.MessageID, 10)}, nil
}

func (a *Adapter) StartListening(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

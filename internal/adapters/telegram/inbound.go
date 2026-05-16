package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/user/notification-hub/internal/adapters"
)

type Poller struct {
	adapter *Adapter
	token   string
	handler adapters.InboundHandler
	offset  int64
	timeout int
}

func NewPoller(adapter *Adapter, token string, handler adapters.InboundHandler) *Poller {
	return &Poller{adapter: adapter, token: token, handler: handler, timeout: 30}
}

func (p *Poller) Start(ctx context.Context) error {
	if p.adapter == nil || p.handler == nil || p.token == "" {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		updates, err := p.getUpdates(ctx)
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}
		for _, update := range updates {
			if update.UpdateID >= p.offset {
				p.offset = update.UpdateID + 1
			}
			if update.Message.Text == "" {
				continue
			}
			inbound := adapters.InboundMessage{
				Platform:          "telegram",
				ChannelID:         strconv.FormatInt(update.Message.Chat.ID, 10),
				PlatformMessageID: strconv.FormatInt(update.Message.MessageID, 10),
				AuthorID:          strconv.FormatInt(update.Message.From.ID, 10),
				AuthorName:        update.Message.From.Username,
				Content:           update.Message.Text,
				Metadata: map[string]any{
					"chat_id":           strconv.FormatInt(update.Message.Chat.ID, 10),
					"message_thread_id": strconv.FormatInt(update.Message.MessageThreadID, 10),
				},
			}
			if err := p.handler.HandleInbound(ctx, inbound); err != nil {
				return err
			}
		}
	}
}

func (p *Poller) getUpdates(ctx context.Context) ([]telegramUpdate, error) {
	values := url.Values{}
	values.Set("timeout", strconv.Itoa(p.timeout))
	if p.offset > 0 {
		values.Set("offset", strconv.FormatInt(p.offset, 10))
	}
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?%s", p.adapter.baseURL, p.token, values.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.adapter.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram getUpdates returned status %d", resp.StatusCode)
	}
	var parsed struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	if !parsed.OK {
		return nil, fmt.Errorf("telegram getUpdates failed")
	}
	return parsed.Result, nil
}

type telegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  struct {
		MessageID       int64  `json:"message_id"`
		MessageThreadID int64  `json:"message_thread_id"`
		Text            string `json:"text"`
		Chat            struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
	} `json:"message"`
}

package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/service"
)

type MessageHandler struct {
	messages service.MessageStore
	channels service.ChannelStore
	pipeline *service.Pipeline
}

type createMessageRequest struct {
	Content      string              `json:"content" binding:"required"`
	Channels     []string            `json:"channels"`
	Priority     domain.Priority     `json:"priority"`
	AIProcessing domain.AIProcessing `json:"ai_processing"`
	Metadata     map[string]any      `json:"metadata"`
}

type notifyRequest struct {
	Text string `json:"text"`
	To   any    `json:"to"`
}

func NewMessageHandler(messages service.MessageStore, channels service.ChannelStore, pipeline *service.Pipeline) *MessageHandler {
	return &MessageHandler{messages: messages, channels: channels, pipeline: pipeline}
}

func (h *MessageHandler) Create(c *gin.Context) {
	var req createMessageRequest
	if !bindJSON(c, &req) {
		return
	}
	msg := domain.NewMessage(domain.DirectionOutbound, req.Content, "api")
	msg.Channels = req.Channels
	msg.Priority = req.Priority
	msg.AIProcessing = req.AIProcessing
	msg.Metadata = req.Metadata
	if msg.Priority == "" {
		msg.Priority = domain.PriorityNormal
	}
	if msg.AIProcessing == "" {
		msg.AIProcessing = domain.AIProcessingNone
	}
	if source, ok := msg.Metadata["source"].(string); ok && source != "" {
		msg.Source = source
	}
	if h.pipeline == nil {
		respondError(c, http.StatusInternalServerError, "pipeline is not configured")
		return
	}
	if err := h.pipeline.Submit(c.Request.Context(), msg); err != nil {
		respondError(c, http.StatusBadGateway, err.Error())
		return
	}
	stored, err := h.messages.Get(c.Request.Context(), msg.ID)
	if err == nil && stored != nil {
		msg = stored
	}
	c.JSON(http.StatusAccepted, msg)
}

func (h *MessageHandler) Notify(c *gin.Context) {
	var req notifyRequest
	if !bindJSON(c, &req) {
		return
	}
	if req.Text == "" {
		respondError(c, http.StatusBadRequest, "text is required")
		return
	}
	channels, err := h.notifyChannels(c, req.To)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	msg := domain.NewMessage(domain.DirectionOutbound, req.Text, "agent")
	msg.Channels = channels
	msg.Priority = domain.PriorityNormal
	msg.AIProcessing = domain.AIProcessingNone
	msg.Metadata = map[string]any{"source": "agent"}
	if h.pipeline == nil {
		respondError(c, http.StatusInternalServerError, "pipeline is not configured")
		return
	}
	if err := h.pipeline.Submit(c.Request.Context(), msg); err != nil {
		respondError(c, http.StatusBadGateway, err.Error())
		return
	}
	stored, err := h.messages.Get(c.Request.Context(), msg.ID)
	if err == nil && stored != nil {
		msg = stored
	}
	c.JSON(http.StatusAccepted, msg)
}

func (h *MessageHandler) notifyChannels(c *gin.Context, to any) ([]string, error) {
	if to == nil {
		return []string{"discord"}, nil
	}
	switch value := to.(type) {
	case string:
		if value == "" {
			return []string{"discord"}, nil
		}
		if value == "all" {
			channels, err := h.channels.ListActive(c.Request.Context())
			if err != nil {
				return nil, err
			}
			out := make([]string, 0, len(channels))
			for _, channel := range channels {
				out = append(out, string(channel.Platform))
			}
			return out, nil
		}
		return []string{value}, nil
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok || text == "" {
				continue
			}
			out = append(out, text)
		}
		if len(out) == 0 {
			return []string{"discord"}, nil
		}
		return out, nil
	default:
		return nil, fmt.Errorf("to must be a string or array of strings")
	}
}

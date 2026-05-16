package domain

import "time"

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusSent       Status = "sent"
	StatusFailed     Status = "failed"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

type AIProcessing string

const (
	AIProcessingNone      AIProcessing = "none"
	AIProcessingSummarize AIProcessing = "summarize"
	AIProcessingTranslate AIProcessing = "translate"
	AIProcessingCustom    AIProcessing = "custom"
)

type Message struct {
	ID                 string            `json:"id"`
	Direction          Direction         `json:"direction"`
	ContentOriginal    string            `json:"content_original"`
	ContentProcessed   string            `json:"content_processed"`
	Source             string            `json:"source"`
	Channels           []string          `json:"channels"`
	Status             Status            `json:"status"`
	Priority           Priority          `json:"priority"`
	AIProcessing       AIProcessing      `json:"ai_processing"`
	Metadata           map[string]any    `json:"metadata"`
	PlatformMessageIDs map[string]string `json:"platform_message_ids"`
	ErrorMessage       string            `json:"error_message,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	SentAt             *time.Time        `json:"sent_at,omitempty"`
}

func NewMessage(direction Direction, content, source string) *Message {
	now := time.Now().UTC()
	return &Message{
		ID:                 NewID(),
		Direction:          direction,
		ContentOriginal:    content,
		ContentProcessed:   content,
		Source:             source,
		Channels:           []string{},
		Status:             StatusPending,
		Priority:           PriorityNormal,
		AIProcessing:       AIProcessingNone,
		Metadata:           map[string]any{},
		PlatformMessageIDs: map[string]string{},
		CreatedAt:          now,
	}
}

func (m *Message) Normalize() {
	if m.ID == "" {
		m.ID = NewID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if m.ContentProcessed == "" {
		m.ContentProcessed = m.ContentOriginal
	}
	if m.Status == "" {
		m.Status = StatusPending
	}
	if m.Priority == "" {
		m.Priority = PriorityNormal
	}
	if m.AIProcessing == "" {
		m.AIProcessing = AIProcessingNone
	}
	if m.Metadata == nil {
		m.Metadata = map[string]any{}
	}
	if m.PlatformMessageIDs == nil {
		m.PlatformMessageIDs = map[string]string{}
	}
	if m.Channels == nil {
		m.Channels = []string{}
	}
}

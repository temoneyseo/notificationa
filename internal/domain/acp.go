package domain

import "time"

const (
	ACPEventVersion                    = "2026-05-17"
	ACPEventTypeChannelInboundAnalyzed = "channel.inbound.analyzed"
)

type ACPEvent struct {
	Version   string         `json:"version"`
	EventType string         `json:"event_type"`
	MessageID string         `json:"message_id"`
	Source    ACPEventSource `json:"source"`
	Routing   ACPRouting     `json:"routing"`
	Analysis  ACPAnalysis    `json:"analysis"`
	Content   ACPContent     `json:"content"`
}

type ACPEventSource struct {
	Platform   string `json:"platform"`
	ChannelID  string `json:"channel_id"`
	AuthorID   string `json:"author_id"`
	AuthorName string `json:"author_name"`
}

type ACPRouting struct {
	ShouldForward bool    `json:"should_forward"`
	Project       string  `json:"project"`
	Agent         string  `json:"agent"`
	Priority      string  `json:"priority"`
	Confidence    float64 `json:"confidence"`
}

type ACPAnalysis struct {
	Intent   string   `json:"intent"`
	Summary  string   `json:"summary"`
	Action   string   `json:"action"`
	Entities []string `json:"entities"`
	Language string   `json:"language"`
}

type ACPContent struct {
	Original   string `json:"original"`
	Normalized string `json:"normalized"`
}

type ACPOutboxStatus string

const (
	ACPOutboxStatusPending    ACPOutboxStatus = "pending"
	ACPOutboxStatusSkipped    ACPOutboxStatus = "skipped"
	ACPOutboxStatusFailed     ACPOutboxStatus = "failed"
	ACPOutboxStatusDispatched ACPOutboxStatus = "dispatched"
)

type ACPOutboxItem struct {
	ID               string          `json:"id"`
	MessageID        string          `json:"message_id"`
	Event            *ACPEvent       `json:"event,omitempty"`
	EventJSON        string          `json:"event_json,omitempty"`
	RawLLMOutput     string          `json:"raw_llm_output"`
	Status           ACPOutboxStatus `json:"status"`
	SkipReason       string          `json:"skip_reason,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	DispatchAttempts int             `json:"dispatch_attempts"`
	LastStatusCode   *int            `json:"last_status_code,omitempty"`
	LastAttemptedAt  *time.Time      `json:"last_attempted_at,omitempty"`
	DispatchedAt     *time.Time      `json:"dispatched_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func NewACPOutboxItem(messageID string, event *ACPEvent, rawLLMOutput string) *ACPOutboxItem {
	now := time.Now().UTC()
	return &ACPOutboxItem{
		ID:           NewID(),
		MessageID:    messageID,
		Event:        event,
		RawLLMOutput: rawLLMOutput,
		Status:       ACPOutboxStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (i *ACPOutboxItem) Normalize() {
	if i.ID == "" {
		i.ID = NewID()
	}
	if i.Status == "" {
		i.Status = ACPOutboxStatusPending
	}
	if i.CreatedAt.IsZero() {
		i.CreatedAt = time.Now().UTC()
	}
	if i.UpdatedAt.IsZero() {
		i.UpdatedAt = i.CreatedAt
	}
}

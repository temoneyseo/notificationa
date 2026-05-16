package domain

import "time"

type WebhookConfig struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	Events          []string   `json:"events"`
	Secret          string     `json:"secret,omitempty"`
	IsActive        bool       `json:"is_active"`
	LastTriggeredAt *time.Time `json:"last_triggered_at,omitempty"`
	LastError       string     `json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func NewWebhookConfig(url string, events []string) *WebhookConfig {
	now := time.Now().UTC()
	return &WebhookConfig{
		ID:        NewID(),
		URL:       url,
		Events:    events,
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (w *WebhookConfig) Normalize() {
	now := time.Now().UTC()
	if w.ID == "" {
		w.ID = NewID()
	}
	if w.Events == nil {
		w.Events = []string{}
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = now
	}
}

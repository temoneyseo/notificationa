package domain

import "time"

type Platform string

const (
	PlatformTelegram Platform = "telegram"
	PlatformDiscord  Platform = "discord"
)

type Channel struct {
	ID        string         `json:"id"`
	Platform  Platform       `json:"platform"`
	Name      string         `json:"name"`
	Config    map[string]any `json:"config"`
	Rules     []Rule         `json:"rules"`
	AIEnabled bool           `json:"ai_enabled"`
	AIPrompt  string         `json:"ai_prompt"`
	IsActive  bool           `json:"is_active"`
	IsDefault bool           `json:"is_default"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func NewChannel(platform Platform, name string) *Channel {
	now := time.Now().UTC()
	return &Channel{
		ID:        NewID(),
		Platform:  platform,
		Name:      name,
		Config:    map[string]any{},
		Rules:     []Rule{},
		IsActive:  true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (c *Channel) Normalize() {
	now := time.Now().UTC()
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.Name == "" {
		c.Name = string(c.Platform)
	}
	if c.Config == nil {
		c.Config = map[string]any{}
	}
	if c.Rules == nil {
		c.Rules = []Rule{}
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
}

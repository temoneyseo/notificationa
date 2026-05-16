package service

import (
	"context"
	"database/sql"

	"github.com/user/notification-hub/internal/domain"
)

type ConfigChannelStore struct {
	channels []domain.Channel
}

func NewConfigChannelStore(channels []domain.Channel) *ConfigChannelStore {
	items := make([]domain.Channel, 0, len(channels))
	for _, ch := range channels {
		ch.Normalize()
		if ch.ID == "" {
			ch.ID = ch.Name
		}
		items = append(items, cloneChannel(ch))
	}
	return &ConfigChannelStore{channels: items}
}

func (s *ConfigChannelStore) Create(context.Context, *domain.Channel) error {
	return ErrConfigOnlyChannels
}

func (s *ConfigChannelStore) Get(_ context.Context, id string) (*domain.Channel, error) {
	for _, ch := range s.channels {
		if ch.ID == id || ch.Name == id {
			copy := cloneChannel(ch)
			return &copy, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (s *ConfigChannelStore) List(context.Context) ([]domain.Channel, error) {
	return cloneChannels(s.channels), nil
}

func (s *ConfigChannelStore) ListActive(context.Context) ([]domain.Channel, error) {
	out := []domain.Channel{}
	for _, ch := range s.channels {
		if ch.IsActive {
			out = append(out, cloneChannel(ch))
		}
	}
	return out, nil
}

func (s *ConfigChannelStore) ListByPlatforms(_ context.Context, platforms []string) ([]domain.Channel, error) {
	wanted := map[string]bool{}
	for _, platform := range platforms {
		wanted[platform] = true
	}
	out := []domain.Channel{}
	for _, ch := range s.channels {
		if ch.IsActive && wanted[string(ch.Platform)] {
			out = append(out, cloneChannel(ch))
		}
	}
	return out, nil
}

func (s *ConfigChannelStore) ListDefault(context.Context) ([]domain.Channel, error) {
	out := []domain.Channel{}
	for _, ch := range s.channels {
		if ch.IsActive && ch.IsDefault {
			out = append(out, cloneChannel(ch))
		}
	}
	return out, nil
}

func (s *ConfigChannelStore) Update(context.Context, *domain.Channel) error {
	return ErrConfigOnlyChannels
}

func (s *ConfigChannelStore) Delete(context.Context, string) error {
	return ErrConfigOnlyChannels
}

func cloneChannels(channels []domain.Channel) []domain.Channel {
	out := make([]domain.Channel, 0, len(channels))
	for _, ch := range channels {
		out = append(out, cloneChannel(ch))
	}
	return out
}

func cloneChannel(ch domain.Channel) domain.Channel {
	copy := ch
	copy.Config = map[string]any{}
	for key, value := range ch.Config {
		copy.Config[key] = value
	}
	copy.Rules = append([]domain.Rule{}, ch.Rules...)
	return copy
}

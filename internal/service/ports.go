package service

import (
	"context"
	"errors"

	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/storage/sqlite"
)

var ErrConfigOnlyChannels = errors.New("channels are managed by config")

type MessageStore interface {
	Create(ctx context.Context, msg *domain.Message) error
	Get(ctx context.Context, id string) (*domain.Message, error)
	Update(ctx context.Context, msg *domain.Message) error
	UpdateStatus(ctx context.Context, id string, status domain.Status, errMsg string) error
	MarkSent(ctx context.Context, msg *domain.Message) error
	ListInbox(ctx context.Context, opts sqlite.MessageListOptions) ([]domain.Message, error)
}

type ChannelStore interface {
	Create(ctx context.Context, ch *domain.Channel) error
	Get(ctx context.Context, id string) (*domain.Channel, error)
	List(ctx context.Context) ([]domain.Channel, error)
	ListActive(ctx context.Context) ([]domain.Channel, error)
	ListByPlatforms(ctx context.Context, platforms []string) ([]domain.Channel, error)
	ListDefault(ctx context.Context) ([]domain.Channel, error)
	Update(ctx context.Context, ch *domain.Channel) error
	Delete(ctx context.Context, id string) error
}

type WebhookStore interface {
	Create(ctx context.Context, hook *domain.WebhookConfig) error
	Get(ctx context.Context, id string) (*domain.WebhookConfig, error)
	List(ctx context.Context) ([]domain.WebhookConfig, error)
	ListActive(ctx context.Context) ([]domain.WebhookConfig, error)
	Update(ctx context.Context, hook *domain.WebhookConfig) error
	Delete(ctx context.Context, id string) error
	MarkTriggered(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id, message string) error
}

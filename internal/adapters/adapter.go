package adapters

import (
	"context"

	"github.com/user/notification-hub/internal/domain"
)

type SendResult struct {
	MessageID string
}

type Adapter interface {
	Platform() string
	Send(ctx context.Context, msg domain.Message, channel domain.Channel) (SendResult, error)
	StartListening(ctx context.Context) error
}

type InboundHandler interface {
	HandleInbound(ctx context.Context, inbound InboundMessage) error
}

type InboundMessage struct {
	Platform          string
	ChannelID         string
	PlatformMessageID string
	AuthorID          string
	AuthorName        string
	Content           string
	Metadata          map[string]any
}

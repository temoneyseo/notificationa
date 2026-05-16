package service

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/user/notification-hub/internal/adapters"
	"github.com/user/notification-hub/internal/domain"
)

type WebhookDispatcher interface {
	Dispatch(ctx context.Context, event string, msg domain.Message) error
}

type AutoReplier interface {
	Handle(ctx context.Context, msg domain.Message) error
}

type InboundDeps struct {
	Messages             MessageStore
	WebhookDispatcher    WebhookDispatcher
	AutoReply            AutoReplier
	LogInboundMessages   bool
	InboundMessageWriter io.Writer
}

type InboundService struct {
	deps InboundDeps
}

func NewInboundService(deps InboundDeps) *InboundService {
	if deps.InboundMessageWriter == nil {
		deps.InboundMessageWriter = os.Stdout
	}
	return &InboundService{deps: deps}
}

func (s *InboundService) HandleInbound(ctx context.Context, inbound adapters.InboundMessage) error {
	msg := domain.NewMessage(domain.DirectionInbound, inbound.Content, inbound.Platform)
	msg.Channels = []string{inbound.Platform}
	msg.Status = domain.StatusSent
	msg.PlatformMessageIDs[inbound.Platform] = inbound.PlatformMessageID
	msg.Metadata = map[string]any{
		"channel_id":  inbound.ChannelID,
		"author_id":   inbound.AuthorID,
		"author_name": inbound.AuthorName,
	}
	for key, value := range inbound.Metadata {
		msg.Metadata[key] = value
	}
	if err := s.deps.Messages.Create(ctx, msg); err != nil {
		return err
	}
	event := fmt.Sprintf("inbound.%s", inbound.Platform)
	if s.deps.LogInboundMessages {
		s.logInbound(event, inbound)
	}
	if s.deps.WebhookDispatcher != nil {
		if err := s.deps.WebhookDispatcher.Dispatch(ctx, event, *msg); err != nil {
			return err
		}
	}
	if s.deps.AutoReply != nil && msg.Source != "auto_reply" {
		if err := s.deps.AutoReply.Handle(ctx, *msg); err != nil {
			return err
		}
	}
	return nil
}

func (s *InboundService) logInbound(event string, inbound adapters.InboundMessage) {
	if inbound.Platform == "telegram" {
		label := fmt.Sprintf("chat:%s", inbound.ChannelID)
		if topic, ok := inbound.Metadata["message_thread_id"].(string); ok && topic != "" && topic != "0" {
			label = fmt.Sprintf("topic:%s", topic)
		}
		_, _ = fmt.Fprintf(
			s.deps.InboundMessageWriter,
			"[%s] %s %s: %s\n",
			time.Now().Format("2006-01-02 15:04:05"),
			event,
			label,
			inbound.Content,
		)
		return
	}

	_, _ = fmt.Fprintf(
		s.deps.InboundMessageWriter,
		"[%s] %s #%s %s: %s\n",
		time.Now().Format("2006-01-02 15:04:05"),
		event,
		inbound.ChannelID,
		inbound.AuthorName,
		inbound.Content,
	)
}

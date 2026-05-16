package service

import (
	"context"

	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/domain"
)

type AutoReplyService struct {
	channels ChannelStore
	pipeline *Pipeline
	ai       ai.Processor
}

func NewAutoReplyService(channels ChannelStore, pipeline *Pipeline, processor ai.Processor) *AutoReplyService {
	if processor == nil {
		processor = ai.NoopProcessor{}
	}
	return &AutoReplyService{channels: channels, pipeline: pipeline, ai: processor}
}

func (s *AutoReplyService) Handle(ctx context.Context, msg domain.Message) error {
	if msg.Direction != domain.DirectionInbound || msg.Source == "auto_reply" {
		return nil
	}
	channels, err := s.channels.ListByPlatforms(ctx, []string{msg.Source})
	if err != nil {
		return err
	}
	for _, channel := range channels {
		if !channel.AIEnabled || channel.AIPrompt == "" {
			continue
		}
		resp, err := s.ai.Process(ctx, ai.Request{
			Content:  msg.ContentOriginal,
			Mode:     domain.AIProcessingCustom,
			Prompt:   channel.AIPrompt,
			Metadata: msg.Metadata,
		})
		if err != nil || resp.Content == "" {
			resp.Content = msg.ContentOriginal
		}
		reply := domain.NewMessage(domain.DirectionOutbound, resp.Content, "auto_reply")
		reply.ContentProcessed = resp.Content
		reply.Channels = []string{string(channel.Platform)}
		reply.AIProcessing = domain.AIProcessingNone
		if s.pipeline != nil {
			if err := s.pipeline.Submit(ctx, reply); err != nil {
				return err
			}
		}
	}
	return nil
}

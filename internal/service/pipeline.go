package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/domain"
)

type PipelineDeps struct {
	Messages   MessageStore
	Channels   ChannelStore
	Adapters   *AdapterRegistry
	AI         ai.Processor
	Cipher     ConfigCipher
	RuleEngine func([]domain.Channel) *RuleEngine
}

type ConfigCipher interface {
	DecryptConfig(map[string]any) (map[string]any, error)
}

type Pipeline struct {
	deps PipelineDeps
}

func NewPipeline(deps PipelineDeps) *Pipeline {
	if deps.RuleEngine == nil {
		deps.RuleEngine = NewRuleEngine
	}
	if deps.AI == nil {
		deps.AI = ai.NoopProcessor{}
	}
	return &Pipeline{deps: deps}
}

func (p *Pipeline) Submit(ctx context.Context, msg *domain.Message) error {
	msg.Direction = domain.DirectionOutbound
	msg.Normalize()
	if err := p.deps.Messages.Create(ctx, msg); err != nil {
		return err
	}
	return p.Process(ctx, msg.ID)
}

func (p *Pipeline) ProcessAsync(ctx context.Context, id string) {
	go func() {
		_ = p.Process(ctx, id)
	}()
}

func (p *Pipeline) Process(ctx context.Context, id string) error {
	msg, err := p.deps.Messages.Get(ctx, id)
	if err != nil {
		return err
	}
	msg.Status = domain.StatusProcessing
	if err := p.deps.Messages.Update(ctx, msg); err != nil {
		return err
	}

	channels, err := p.deps.Channels.ListActive(ctx)
	if err != nil {
		_ = p.deps.Messages.UpdateStatus(ctx, msg.ID, domain.StatusFailed, err.Error())
		return err
	}
	targetPlatforms := p.deps.RuleEngine(channels).ResolveChannels(msg)
	if len(targetPlatforms) == 0 {
		err := fmt.Errorf("no target channels configured")
		_ = p.deps.Messages.UpdateStatus(ctx, msg.ID, domain.StatusFailed, err.Error())
		return err
	}
	msg.Channels = targetPlatforms

	processed, err := p.deps.AI.Process(ctx, ai.Request{
		Content:  msg.ContentOriginal,
		Mode:     msg.AIProcessing,
		Metadata: msg.Metadata,
	})
	if err != nil {
		processed = ai.Response{Content: msg.ContentOriginal, Fallback: true}
	}
	msg.ContentProcessed = processed.Content

	targetChannels, err := p.deps.Channels.ListByPlatforms(ctx, targetPlatforms)
	if err != nil {
		_ = p.deps.Messages.UpdateStatus(ctx, msg.ID, domain.StatusFailed, err.Error())
		return err
	}

	sendErrors := []string{}
	for _, channel := range targetChannels {
		if p.deps.Cipher != nil {
			decrypted, err := p.deps.Cipher.DecryptConfig(channel.Config)
			if err != nil {
				sendErrors = append(sendErrors, fmt.Sprintf("%s: %v", channel.Platform, err))
				continue
			}
			channel.Config = decrypted
		}
		adapter, err := p.deps.Adapters.Get(string(channel.Platform))
		if err != nil {
			sendErrors = append(sendErrors, err.Error())
			continue
		}
		result, err := adapter.Send(ctx, *msg, channel)
		if err != nil {
			sendErrors = append(sendErrors, fmt.Sprintf("%s: %v", channel.Platform, err))
			continue
		}
		msg.PlatformMessageIDs[string(channel.Platform)] = result.MessageID
	}
	if len(sendErrors) > 0 {
		msg.Status = domain.StatusFailed
		msg.ErrorMessage = strings.Join(sendErrors, "; ")
		if err := p.deps.Messages.Update(ctx, msg); err != nil {
			return err
		}
		return errors.New(msg.ErrorMessage)
	}
	now := time.Now().UTC()
	msg.SentAt = &now
	msg.Status = domain.StatusSent
	msg.ErrorMessage = ""
	return p.deps.Messages.Update(ctx, msg)
}

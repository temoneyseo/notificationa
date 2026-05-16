package service

import (
	"context"
	"testing"

	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/domain"
)

func TestPipelineProcessesAndSendsToMultipleChannels(t *testing.T) {
	msgRepo := newMemoryMessageRepo()
	channelRepo := &memoryChannelRepo{channels: []domain.Channel{
		*domain.NewChannel(domain.PlatformTelegram, "tg"),
		*domain.NewChannel(domain.PlatformDiscord, "dc"),
	}}
	adapterRegistry := NewAdapterRegistry()
	tg := &fakeAdapter{platform: "telegram"}
	dc := &fakeAdapter{platform: "discord"}
	adapterRegistry.Register(tg)
	adapterRegistry.Register(dc)

	pipeline := NewPipeline(PipelineDeps{
		Messages:   msgRepo,
		Channels:   channelRepo,
		Adapters:   adapterRegistry,
		AI:         ai.SafeProcessor{Processor: ai.NoopProcessor{}},
		RuleEngine: func(channels []domain.Channel) *RuleEngine { return NewRuleEngine(channels) },
	})

	msg := domain.NewMessage(domain.DirectionOutbound, "hello", "api")
	msg.Channels = []string{"telegram", "discord"}
	if err := msgRepo.Create(context.Background(), msg); err != nil {
		t.Fatal(err)
	}

	if err := pipeline.Process(context.Background(), msg.ID); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(tg.sent) != 1 || len(dc.sent) != 1 {
		t.Fatalf("sent tg=%d dc=%d", len(tg.sent), len(dc.sent))
	}
	got, _ := msgRepo.Get(context.Background(), msg.ID)
	if got.Status != domain.StatusSent {
		t.Fatalf("status = %s", got.Status)
	}
	if got.PlatformMessageIDs["telegram"] == "" || got.PlatformMessageIDs["discord"] == "" {
		t.Fatalf("platform ids = %+v", got.PlatformMessageIDs)
	}
}

package service

import (
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func TestRuleEngineUserSpecifiedChannelsWin(t *testing.T) {
	engine := NewRuleEngine([]domain.Channel{
		{
			Platform:  domain.PlatformTelegram,
			IsActive:  true,
			IsDefault: true,
			Rules: []domain.Rule{{
				Type:     domain.RuleTypeKeyword,
				Pattern:  "urgent",
				Channels: []string{"discord"},
			}},
		},
	})
	msg := domain.NewMessage(domain.DirectionOutbound, "urgent issue", "api")
	msg.Channels = []string{"telegram"}

	channels := engine.ResolveChannels(msg)
	if len(channels) != 1 || channels[0] != "telegram" {
		t.Fatalf("channels = %+v", channels)
	}
}

func TestRuleEngineMatchesKeywordThenDefault(t *testing.T) {
	engine := NewRuleEngine([]domain.Channel{
		{
			Platform:  domain.PlatformTelegram,
			IsActive:  true,
			IsDefault: true,
		},
		{
			Platform: domain.PlatformDiscord,
			IsActive: true,
			Rules: []domain.Rule{{
				Type:     domain.RuleTypeKeyword,
				Pattern:  "urgent|紧急",
				Channels: []string{"discord"},
			}},
		},
	})

	msg := domain.NewMessage(domain.DirectionOutbound, "紧急：CPU high", "api")
	if got := engine.ResolveChannels(msg); len(got) != 1 || got[0] != "discord" {
		t.Fatalf("keyword channels = %+v", got)
	}

	msg = domain.NewMessage(domain.DirectionOutbound, "daily report", "api")
	if got := engine.ResolveChannels(msg); len(got) != 1 || got[0] != "telegram" {
		t.Fatalf("default channels = %+v", got)
	}
}

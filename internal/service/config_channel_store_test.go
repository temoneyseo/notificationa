package service

import (
	"context"
	"errors"
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func TestConfigChannelStoreListsAndFindsChannels(t *testing.T) {
	store := NewConfigChannelStore([]domain.Channel{
		{
			ID:        "telegram-main",
			Platform:  domain.PlatformTelegram,
			Name:      "telegram-main",
			Config:    map[string]any{"bot_token": "telegram-token", "chat_id": "-100"},
			IsActive:  true,
			IsDefault: true,
		},
		{
			ID:       "discord-main",
			Platform: domain.PlatformDiscord,
			Name:     "discord-main",
			Config:   map[string]any{"bot_token": "discord-token", "channel_id": "123"},
			IsActive: true,
		},
	})

	all, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("List length = %d", len(all))
	}

	defaults, err := store.ListDefault(context.Background())
	if err != nil {
		t.Fatalf("ListDefault: %v", err)
	}
	if len(defaults) != 1 || defaults[0].Platform != domain.PlatformTelegram {
		t.Fatalf("unexpected defaults: %+v", defaults)
	}

	telegram, err := store.ListByPlatforms(context.Background(), []string{"telegram"})
	if err != nil {
		t.Fatalf("ListByPlatforms: %v", err)
	}
	if len(telegram) != 1 || telegram[0].Config["bot_token"] != "telegram-token" {
		t.Fatalf("unexpected telegram channels: %+v", telegram)
	}

	got, err := store.Get(context.Background(), "discord-main")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Platform != domain.PlatformDiscord {
		t.Fatalf("got platform = %s", got.Platform)
	}
}

func TestConfigChannelStoreRejectsMutations(t *testing.T) {
	store := NewConfigChannelStore(nil)
	if err := store.Create(context.Background(), domain.NewChannel(domain.PlatformTelegram, "x")); !errors.Is(err, ErrConfigOnlyChannels) {
		t.Fatalf("Create error = %v", err)
	}
	if err := store.Update(context.Background(), domain.NewChannel(domain.PlatformTelegram, "x")); !errors.Is(err, ErrConfigOnlyChannels) {
		t.Fatalf("Update error = %v", err)
	}
	if err := store.Delete(context.Background(), "x"); !errors.Is(err, ErrConfigOnlyChannels) {
		t.Fatalf("Delete error = %v", err)
	}
}

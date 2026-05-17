package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/user/notification-hub/internal/adapters"
	"github.com/user/notification-hub/internal/domain"
)

func TestInboundServiceStoresInboundAndTriggersSideEffects(t *testing.T) {
	messages := newMemoryMessageRepo()
	dispatcher := &fakeWebhookDispatcher{}
	auto := &fakeAutoReply{}
	svc := NewInboundService(InboundDeps{
		Messages:          messages,
		WebhookDispatcher: dispatcher,
		AutoReply:         auto,
	})

	err := svc.HandleInbound(context.Background(), adapters.InboundMessage{
		Platform:          "telegram",
		PlatformMessageID: "m1",
		AuthorID:          "u1",
		Content:           "hello",
		Metadata:          map[string]any{"chat_id": "42"},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	items, err := messages.ListInbox(context.Background(), nilListOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Direction != domain.DirectionInbound || items[0].Source != "telegram" {
		t.Fatalf("items = %+v", items)
	}
	if dispatcher.event != "inbound.telegram" || !auto.called {
		t.Fatalf("dispatcher event=%q auto called=%v", dispatcher.event, auto.called)
	}
}

func TestInboundServiceCallsACPAfterWebhookAndBeforeAutoReply(t *testing.T) {
	messages := newMemoryMessageRepo()
	calls := []string{}
	dispatcher := &fakeWebhookDispatcher{onDispatch: func() { calls = append(calls, "webhook") }}
	acp := &fakeInboundACPForwarder{onForward: func() { calls = append(calls, "acp") }}
	auto := &fakeAutoReply{onHandle: func() { calls = append(calls, "auto") }}
	svc := NewInboundService(InboundDeps{
		Messages:          messages,
		WebhookDispatcher: dispatcher,
		ACPForwarder:      acp,
		AutoReply:         auto,
	})

	err := svc.HandleInbound(context.Background(), adapters.InboundMessage{
		Platform:          "telegram",
		ChannelID:         "-100",
		PlatformMessageID: "m1",
		AuthorID:          "u1",
		AuthorName:        "alice",
		Content:           "hello",
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	want := []string{"webhook", "acp", "auto"}
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v", calls)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("calls = %+v", calls)
		}
	}
	if acp.msg.ID == "" || acp.msg.ContentOriginal != "hello" || acp.msg.Metadata["channel_id"] != "-100" {
		t.Fatalf("unexpected acp message: %+v", acp.msg)
	}
}

func TestInboundServiceACPErrorDoesNotBlockAutoReply(t *testing.T) {
	messages := newMemoryMessageRepo()
	acp := &fakeInboundACPForwarder{err: errors.New("acp down")}
	auto := &fakeAutoReply{}
	svc := NewInboundService(InboundDeps{
		Messages:     messages,
		ACPForwarder: acp,
		AutoReply:    auto,
	})

	err := svc.HandleInbound(context.Background(), adapters.InboundMessage{
		Platform: "discord",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("HandleInbound should not return ACP error: %v", err)
	}
	if !auto.called {
		t.Fatal("auto reply should still be called")
	}
}

func TestInboundServiceNilACPKeepsExistingBehavior(t *testing.T) {
	messages := newMemoryMessageRepo()
	auto := &fakeAutoReply{}
	svc := NewInboundService(InboundDeps{
		Messages:  messages,
		AutoReply: auto,
	})

	err := svc.HandleInbound(context.Background(), adapters.InboundMessage{
		Platform: "telegram",
		Content:  "hello",
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}
	if !auto.called {
		t.Fatal("auto reply should be called")
	}
}

func TestInboundServiceLogsTelegramTopicWithoutAuthor(t *testing.T) {
	messages := newMemoryMessageRepo()
	var out bytes.Buffer
	svc := NewInboundService(InboundDeps{
		Messages:             messages,
		LogInboundMessages:   true,
		InboundMessageWriter: &out,
	})

	err := svc.HandleInbound(context.Background(), adapters.InboundMessage{
		Platform:          "telegram",
		ChannelID:         "-100123",
		PlatformMessageID: "m1",
		AuthorID:          "u1",
		AuthorName:        "alice",
		Content:           "hello topic",
		Metadata: map[string]any{
			"message_thread_id": "42",
		},
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	line := out.String()
	for _, want := range []string{"inbound.telegram", "topic:42", "hello topic"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "alice") {
		t.Fatalf("telegram log line should not include author: %q", line)
	}
}

func TestInboundServiceLogsTelegramChatWithoutAuthorWhenNoTopic(t *testing.T) {
	messages := newMemoryMessageRepo()
	var out bytes.Buffer
	svc := NewInboundService(InboundDeps{
		Messages:             messages,
		LogInboundMessages:   true,
		InboundMessageWriter: &out,
	})

	err := svc.HandleInbound(context.Background(), adapters.InboundMessage{
		Platform:          "telegram",
		ChannelID:         "-100123",
		PlatformMessageID: "m1",
		AuthorName:        "alice",
		Content:           "hello chat",
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	line := out.String()
	for _, want := range []string{"inbound.telegram", "chat:-100123", "hello chat"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
	if strings.Contains(line, "alice") {
		t.Fatalf("telegram log line should not include author: %q", line)
	}
}

func TestInboundServiceLogsInboundMessage(t *testing.T) {
	messages := newMemoryMessageRepo()
	var out bytes.Buffer
	svc := NewInboundService(InboundDeps{
		Messages:             messages,
		LogInboundMessages:   true,
		InboundMessageWriter: &out,
	})

	err := svc.HandleInbound(context.Background(), adapters.InboundMessage{
		Platform:          "discord",
		ChannelID:         "1504737600336826370",
		PlatformMessageID: "m1",
		AuthorID:          "u1",
		AuthorName:        "alice",
		Content:           "hello from discord",
	})
	if err != nil {
		t.Fatalf("HandleInbound: %v", err)
	}

	line := out.String()
	for _, want := range []string{"inbound.discord", "1504737600336826370", "alice", "hello from discord"} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
}

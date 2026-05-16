package service

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/user/notification-hub/internal/adapters"
	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/storage/sqlite"
)

type memoryMessageRepo struct {
	mu       sync.Mutex
	messages map[string]*domain.Message
}

func newMemoryMessageRepo() *memoryMessageRepo {
	return &memoryMessageRepo{messages: map[string]*domain.Message{}}
}

func (r *memoryMessageRepo) Create(_ context.Context, msg *domain.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *msg
	r.messages[msg.ID] = &cp
	return nil
}

func (r *memoryMessageRepo) Get(_ context.Context, id string) (*domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg, ok := r.messages[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *msg
	return &cp, nil
}

func (r *memoryMessageRepo) Update(_ context.Context, msg *domain.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *msg
	r.messages[msg.ID] = &cp
	return nil
}

func (r *memoryMessageRepo) UpdateStatus(_ context.Context, id string, status domain.Status, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	msg := r.messages[id]
	msg.Status = status
	msg.ErrorMessage = errMsg
	return nil
}

func (r *memoryMessageRepo) MarkSent(ctx context.Context, msg *domain.Message) error {
	msg.Status = domain.StatusSent
	return r.Update(ctx, msg)
}

func (r *memoryMessageRepo) ListInbox(_ context.Context, _ sqlite.MessageListOptions) ([]domain.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []domain.Message{}
	for _, msg := range r.messages {
		if msg.Direction == domain.DirectionInbound {
			items = append(items, *msg)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}

type memoryChannelRepo struct {
	channels []domain.Channel
}

func (r *memoryChannelRepo) Create(_ context.Context, ch *domain.Channel) error {
	r.channels = append(r.channels, *ch)
	return nil
}

func (r *memoryChannelRepo) Get(_ context.Context, id string) (*domain.Channel, error) {
	for _, ch := range r.channels {
		if ch.ID == id {
			cp := ch
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (r *memoryChannelRepo) List(context.Context) ([]domain.Channel, error) {
	return append([]domain.Channel(nil), r.channels...), nil
}

func (r *memoryChannelRepo) ListActive(context.Context) ([]domain.Channel, error) {
	active := []domain.Channel{}
	for _, ch := range r.channels {
		if ch.IsActive {
			active = append(active, ch)
		}
	}
	return active, nil
}

func (r *memoryChannelRepo) ListByPlatforms(_ context.Context, platforms []string) ([]domain.Channel, error) {
	want := map[string]struct{}{}
	for _, p := range platforms {
		want[p] = struct{}{}
	}
	items := []domain.Channel{}
	for _, ch := range r.channels {
		if _, ok := want[string(ch.Platform)]; ok && ch.IsActive {
			items = append(items, ch)
		}
	}
	return items, nil
}

func (r *memoryChannelRepo) ListDefault(context.Context) ([]domain.Channel, error) {
	items := []domain.Channel{}
	for _, ch := range r.channels {
		if ch.IsDefault && ch.IsActive {
			items = append(items, ch)
		}
	}
	return items, nil
}

func (r *memoryChannelRepo) Update(_ context.Context, ch *domain.Channel) error {
	for i := range r.channels {
		if r.channels[i].ID == ch.ID {
			r.channels[i] = *ch
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (r *memoryChannelRepo) Delete(_ context.Context, id string) error {
	for i := range r.channels {
		if r.channels[i].ID == id {
			r.channels = append(r.channels[:i], r.channels[i+1:]...)
			return nil
		}
	}
	return nil
}

type fakeAdapter struct {
	platform string
	sent     []domain.Message
}

func (a *fakeAdapter) Platform() string { return a.platform }

func (a *fakeAdapter) Send(_ context.Context, msg domain.Message, _ domain.Channel) (adapters.SendResult, error) {
	a.sent = append(a.sent, msg)
	return adapters.SendResult{MessageID: a.platform + "-message"}, nil
}

func (a *fakeAdapter) StartListening(context.Context) error { return nil }

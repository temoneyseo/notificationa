package service

import (
	"context"

	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/storage/sqlite"
)

type memoryWebhookRepo struct {
	hooks     []domain.WebhookConfig
	triggered map[string]bool
	failed    map[string]string
}

func (r *memoryWebhookRepo) ensureMaps() {
	if r.triggered == nil {
		r.triggered = map[string]bool{}
	}
	if r.failed == nil {
		r.failed = map[string]string{}
	}
}

func (r *memoryWebhookRepo) Create(_ context.Context, hook *domain.WebhookConfig) error {
	r.hooks = append(r.hooks, *hook)
	return nil
}

func (r *memoryWebhookRepo) Get(_ context.Context, id string) (*domain.WebhookConfig, error) {
	for _, hook := range r.hooks {
		if hook.ID == id {
			cp := hook
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *memoryWebhookRepo) List(context.Context) ([]domain.WebhookConfig, error) {
	return append([]domain.WebhookConfig(nil), r.hooks...), nil
}

func (r *memoryWebhookRepo) ListActive(context.Context) ([]domain.WebhookConfig, error) {
	items := []domain.WebhookConfig{}
	for _, hook := range r.hooks {
		if hook.IsActive {
			items = append(items, hook)
		}
	}
	return items, nil
}

func (r *memoryWebhookRepo) Update(_ context.Context, hook *domain.WebhookConfig) error {
	for i := range r.hooks {
		if r.hooks[i].ID == hook.ID {
			r.hooks[i] = *hook
			return nil
		}
	}
	return nil
}

func (r *memoryWebhookRepo) Delete(_ context.Context, id string) error {
	for i := range r.hooks {
		if r.hooks[i].ID == id {
			r.hooks = append(r.hooks[:i], r.hooks[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *memoryWebhookRepo) MarkTriggered(_ context.Context, id string) error {
	r.ensureMaps()
	r.triggered[id] = true
	return nil
}

func (r *memoryWebhookRepo) MarkFailed(_ context.Context, id, message string) error {
	r.ensureMaps()
	r.failed[id] = message
	return nil
}

type fakeWebhookDispatcher struct {
	event string
	msg   domain.Message
}

func (d *fakeWebhookDispatcher) Dispatch(_ context.Context, event string, msg domain.Message) error {
	d.event = event
	d.msg = msg
	return nil
}

type fakeAutoReply struct {
	called bool
	msg    domain.Message
}

func (a *fakeAutoReply) Handle(_ context.Context, msg domain.Message) error {
	a.called = true
	a.msg = msg
	return nil
}

func nilListOptions() sqlite.MessageListOptions {
	return sqlite.MessageListOptions{}
}

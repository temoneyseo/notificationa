package service

import (
	"context"

	"github.com/user/notification-hub/internal/domain"
)

type ACPDispatchResult struct {
	StatusCode int
}

type ACPClient interface {
	Send(ctx context.Context, event domain.ACPEvent) (ACPDispatchResult, error)
}

type ACPDispatcher interface {
	Dispatch(ctx context.Context, event domain.ACPEvent) (ACPDispatchResult, error)
}

type ACPClientDispatcher struct {
	client ACPClient
}

func NewACPDispatcher(client ACPClient) *ACPClientDispatcher {
	return &ACPClientDispatcher{client: client}
}

func (d *ACPClientDispatcher) Dispatch(ctx context.Context, event domain.ACPEvent) (ACPDispatchResult, error) {
	if d == nil || d.client == nil {
		return ACPDispatchResult{}, nil
	}
	return d.client.Send(ctx, event)
}

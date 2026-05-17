package service

import (
	"context"

	"github.com/user/notification-hub/internal/domain"
)

type ACPForwardConfig struct {
	Enabled        bool
	EndpointURL    string
	MinConfidence  float64
	AllowedIntents []string
}

type ACPForwardDecision struct {
	Allow  bool
	Reason string
}

func DecideACPForward(cfg ACPForwardConfig, event *domain.ACPEvent) ACPForwardDecision {
	if !cfg.Enabled {
		return ACPForwardDecision{Reason: "acp_disabled"}
	}
	if cfg.EndpointURL == "" {
		return ACPForwardDecision{Reason: "missing_endpoint"}
	}
	if event == nil {
		return ACPForwardDecision{Reason: "missing_event"}
	}
	if !event.Routing.ShouldForward {
		return ACPForwardDecision{Reason: "should_forward_false"}
	}
	if event.Routing.Confidence < cfg.MinConfidence {
		return ACPForwardDecision{Reason: "low_confidence"}
	}
	if len(cfg.AllowedIntents) > 0 && !acpIntentAllowed(cfg.AllowedIntents, event.Analysis.Intent) {
		return ACPForwardDecision{Reason: "intent_not_allowed"}
	}
	return ACPForwardDecision{Allow: true}
}

func acpIntentAllowed(allowed []string, intent string) bool {
	for _, item := range allowed {
		if item == intent {
			return true
		}
	}
	return false
}

type ACPMessageAnalyzer interface {
	Analyze(ctx context.Context, msg domain.Message) (ACPAnalysisResult, error)
}

type ACPOutbox interface {
	Create(ctx context.Context, item *domain.ACPOutboxItem) error
	Get(ctx context.Context, id string) (*domain.ACPOutboxItem, error)
	MarkSkipped(ctx context.Context, id string, reason string) error
	MarkFailed(ctx context.Context, id string, message string, statusCode *int) error
	MarkDispatched(ctx context.Context, id string, statusCode int) error
}

type ACPForwarderDeps struct {
	Analyzer   ACPMessageAnalyzer
	Outbox     ACPOutbox
	Dispatcher ACPDispatcher
	Config     ACPForwardConfig
}

type ACPForwarder struct {
	deps ACPForwarderDeps
}

func NewACPForwarder(deps ACPForwarderDeps) *ACPForwarder {
	return &ACPForwarder{deps: deps}
}

func (f *ACPForwarder) ForwardInbound(ctx context.Context, msg domain.Message) error {
	if f == nil || f.deps.Analyzer == nil || f.deps.Outbox == nil {
		return nil
	}
	result, err := f.deps.Analyzer.Analyze(ctx, msg)
	item := domain.NewACPOutboxItem(msg.ID, result.Event, result.RawOutput)
	if err != nil {
		item.Status = domain.ACPOutboxStatusFailed
		item.ErrorMessage = err.Error()
		_ = f.deps.Outbox.Create(ctx, item)
		return nil
	}
	if createErr := f.deps.Outbox.Create(ctx, item); createErr != nil {
		return nil
	}
	decision := DecideACPForward(f.deps.Config, result.Event)
	if !decision.Allow {
		_ = f.deps.Outbox.MarkSkipped(ctx, item.ID, decision.Reason)
		return nil
	}
	if f.deps.Dispatcher == nil {
		_ = f.deps.Outbox.MarkSkipped(ctx, item.ID, "missing_dispatcher")
		return nil
	}
	dispatchResult, err := f.deps.Dispatcher.Dispatch(ctx, *result.Event)
	if err != nil {
		var statusCode *int
		if dispatchResult.StatusCode != 0 {
			statusCode = &dispatchResult.StatusCode
		}
		_ = f.deps.Outbox.MarkFailed(ctx, item.ID, err.Error(), statusCode)
		return nil
	}
	_ = f.deps.Outbox.MarkDispatched(ctx, item.ID, dispatchResult.StatusCode)
	return nil
}

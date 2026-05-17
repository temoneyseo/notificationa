package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/domain"
)

type processorFunc func(context.Context, ai.Request) (ai.Response, error)

func (f processorFunc) Process(ctx context.Context, req ai.Request) (ai.Response, error) {
	return f(ctx, req)
}

func TestACPAnalyzerBuildsEventFromValidatedJSONAndWhitelistedContext(t *testing.T) {
	msg := domain.NewMessage(domain.DirectionInbound, "我看 README 缺 API 示例", "telegram")
	msg.ID = "msg_123"
	msg.Metadata = map[string]any{
		"channel_id":      "-100",
		"author_id":       "u1",
		"author_name":     "alice",
		"bot_token":       "secret-token",
		"webhook_secret":  "webhook-secret",
		"openai_api_key":  "openai-secret",
		"decrypted_token": "decrypted-secret",
	}
	var seen ai.Request
	analyzer := NewACPAnalyzer(ACPAnalyzerConfig{
		Processor: processorFunc(func(_ context.Context, req ai.Request) (ai.Response, error) {
			seen = req
			return ai.Response{Content: `{
				"should_forward": true,
				"intent": "docs_request",
				"project": "docs-team",
				"agent": "triage_agent",
				"priority": "high",
				"confidence": 0.92,
				"summary": "README lacks quick API send examples.",
				"action": "Add curl examples to README.",
				"entities": ["README", "API", "curl"],
				"language": "zh",
				"normalized_content": "README is missing quick API message examples."
			}`}, nil
		}),
		DefaultProject: "notification",
		DefaultAgent:   "triage",
	})

	result, err := analyzer.Analyze(context.Background(), *msg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if seen.Mode != domain.AIProcessingCustom {
		t.Fatalf("unexpected AI request: %+v", seen)
	}
	for _, want := range []string{`"content":"我看 README 缺 API 示例"`, `"platform":"telegram"`, `"channel_id":"-100"`, `"message_id":"msg_123"`} {
		if !strings.Contains(seen.Content, want) {
			t.Fatalf("AI content %q missing %q", seen.Content, want)
		}
	}
	for _, key := range []string{"platform", "channel_id", "author_id", "author_name", "message_id"} {
		if _, ok := seen.Metadata[key]; !ok {
			t.Fatalf("AI metadata missing %q: %+v", key, seen.Metadata)
		}
	}
	for _, key := range []string{"bot_token", "webhook_secret", "openai_api_key", "decrypted_token"} {
		if _, ok := seen.Metadata[key]; ok {
			t.Fatalf("AI metadata leaked %q: %+v", key, seen.Metadata)
		}
		if strings.Contains(seen.Prompt, key) {
			t.Fatalf("AI prompt leaked key %q: %s", key, seen.Prompt)
		}
		if strings.Contains(seen.Content, key) {
			t.Fatalf("AI content leaked key %q: %s", key, seen.Content)
		}
	}
	event := result.Event
	if event.Version != domain.ACPEventVersion || event.EventType != domain.ACPEventTypeChannelInboundAnalyzed {
		t.Fatalf("unexpected event constants: %+v", event)
	}
	if event.MessageID != msg.ID || event.Source.Platform != "telegram" || event.Source.ChannelID != "-100" {
		t.Fatalf("unexpected source: %+v", event)
	}
	if !event.Routing.ShouldForward || event.Routing.Project != "docs-team" || event.Routing.Agent != "triage_agent" {
		t.Fatalf("unexpected routing: %+v", event.Routing)
	}
	if event.Analysis.Intent != "docs_request" || event.Analysis.Entities[2] != "curl" {
		t.Fatalf("unexpected analysis: %+v", event.Analysis)
	}
	if event.Content.Original != msg.ContentOriginal || event.Content.Normalized == "" {
		t.Fatalf("unexpected content: %+v", event.Content)
	}
}

func TestACPAnalyzerRejectsMalformedJSON(t *testing.T) {
	msg := domain.NewMessage(domain.DirectionInbound, "hello", "discord")
	analyzer := NewACPAnalyzer(ACPAnalyzerConfig{
		Processor: processorFunc(func(context.Context, ai.Request) (ai.Response, error) {
			return ai.Response{Content: "not-json"}, nil
		}),
		DefaultProject: "notification",
		DefaultAgent:   "triage",
	})

	result, err := analyzer.Analyze(context.Background(), *msg)
	if err == nil {
		t.Fatal("Analyze should reject malformed JSON")
	}
	if result.Event != nil {
		t.Fatalf("malformed output should not produce event: %+v", result.Event)
	}
	if result.RawOutput != "not-json" {
		t.Fatalf("raw output = %q", result.RawOutput)
	}
}

func TestACPAnalyzerValidatesAndSanitizesFields(t *testing.T) {
	msg := domain.NewMessage(domain.DirectionInbound, "need help", "discord")
	analyzer := NewACPAnalyzer(ACPAnalyzerConfig{
		Processor: processorFunc(func(context.Context, ai.Request) (ai.Response, error) {
			return ai.Response{Content: `{
				"should_forward": true,
				"intent": "support_request",
				"project": "../secret project",
				"agent": "agent token\nleak",
				"priority": "normal",
				"confidence": 0.61,
				"summary": "Need support.",
				"action": "Triage support request.",
				"entities": ["support", 42, "ticket"],
				"language": "en",
				"normalized_content": "Need help."
			}`}, nil
		}),
		DefaultProject: "notification",
		DefaultAgent:   "triage",
	})

	result, err := analyzer.Analyze(context.Background(), *msg)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.Event.Routing.Project != "notification" || result.Event.Routing.Agent != "triage" {
		t.Fatalf("unsafe project/agent should fallback: %+v", result.Event.Routing)
	}
	if len(result.Event.Analysis.Entities) != 2 || result.Event.Analysis.Entities[1] != "ticket" {
		t.Fatalf("entities not sanitized: %+v", result.Event.Analysis.Entities)
	}
}

func TestACPAnalyzerRejectsInvalidRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "missing intent", body: `{"should_forward":true,"priority":"normal","confidence":0.5,"summary":"s","action":"a"}`},
		{name: "bad confidence", body: `{"should_forward":true,"intent":"x","priority":"normal","confidence":1.5,"summary":"s","action":"a"}`},
		{name: "bad priority", body: `{"should_forward":true,"intent":"x","priority":"urgent","confidence":0.5,"summary":"s","action":"a"}`},
		{name: "missing summary", body: `{"should_forward":true,"intent":"x","priority":"normal","confidence":0.5,"action":"a"}`},
		{name: "missing action", body: `{"should_forward":true,"intent":"x","priority":"normal","confidence":0.5,"summary":"s"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := NewACPAnalyzer(ACPAnalyzerConfig{
				Processor: processorFunc(func(context.Context, ai.Request) (ai.Response, error) {
					return ai.Response{Content: tc.body}, nil
				}),
				DefaultProject: "notification",
				DefaultAgent:   "triage",
			})

			if _, err := analyzer.Analyze(context.Background(), *domain.NewMessage(domain.DirectionInbound, "hello", "telegram")); err == nil {
				t.Fatal("Analyze should fail validation")
			}
		})
	}
}

func TestACPForwardDecision(t *testing.T) {
	event := &domain.ACPEvent{
		Routing:  domain.ACPRouting{ShouldForward: true, Confidence: 0.86},
		Analysis: domain.ACPAnalysis{Intent: "docs_request"},
	}
	tests := []struct {
		name   string
		cfg    ACPForwardConfig
		event  *domain.ACPEvent
		allow  bool
		reason string
	}{
		{name: "allowed", cfg: ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp", MinConfidence: 0.8, AllowedIntents: []string{"docs_request"}}, event: event, allow: true},
		{name: "disabled", cfg: ACPForwardConfig{Enabled: false, EndpointURL: "https://example.test/acp"}, event: event, reason: "acp_disabled"},
		{name: "missing endpoint", cfg: ACPForwardConfig{Enabled: true}, event: event, reason: "missing_endpoint"},
		{name: "should not forward", cfg: ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp"}, event: &domain.ACPEvent{Routing: domain.ACPRouting{Confidence: 0.9}, Analysis: domain.ACPAnalysis{Intent: "docs_request"}}, reason: "should_forward_false"},
		{name: "low confidence", cfg: ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp", MinConfidence: 0.9}, event: event, reason: "low_confidence"},
		{name: "disallowed intent", cfg: ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp", AllowedIntents: []string{"incident"}}, event: event, reason: "intent_not_allowed"},
		{name: "empty allowed intents allows any", cfg: ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp"}, event: event, allow: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideACPForward(tt.cfg, tt.event)
			if got.Allow != tt.allow || got.Reason != tt.reason {
				t.Fatalf("decision = %+v", got)
			}
		})
	}
}

func TestACPForwarderDispatchesAndMarksOutbox(t *testing.T) {
	event := testACPEvent()
	analyzer := &fakeACPAnalyzer{result: ACPAnalysisResult{Event: event, RawOutput: `{"ok":true}`}}
	outbox := &fakeACPOutbox{}
	dispatcher := &fakeACPDispatcher{result: ACPDispatchResult{StatusCode: 202}}
	forwarder := NewACPForwarder(ACPForwarderDeps{
		Analyzer:   analyzer,
		Outbox:     outbox,
		Dispatcher: dispatcher,
		Config:     ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp", MinConfidence: 0.8},
	})

	err := forwarder.ForwardInbound(context.Background(), *domain.NewMessage(domain.DirectionInbound, "hello", "telegram"))
	if err != nil {
		t.Fatalf("ForwardInbound: %v", err)
	}

	if len(outbox.created) != 1 || outbox.created[0].Event != event {
		t.Fatalf("unexpected created outbox items: %+v", outbox.created)
	}
	if len(dispatcher.events) != 1 || dispatcher.events[0].MessageID != event.MessageID {
		t.Fatalf("dispatcher events = %+v", dispatcher.events)
	}
	if outbox.dispatchedID != outbox.created[0].ID || outbox.dispatchedStatus != 202 {
		t.Fatalf("outbox not marked dispatched: %+v", outbox)
	}
}

func TestACPForwarderMarksMalformedAnalysisFailed(t *testing.T) {
	analyzer := &fakeACPAnalyzer{result: ACPAnalysisResult{RawOutput: "not-json"}, err: errors.New("parse acp analysis json")}
	outbox := &fakeACPOutbox{}
	dispatcher := &fakeACPDispatcher{}
	forwarder := NewACPForwarder(ACPForwarderDeps{
		Analyzer:   analyzer,
		Outbox:     outbox,
		Dispatcher: dispatcher,
		Config:     ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp"},
	})

	if err := forwarder.ForwardInbound(context.Background(), *domain.NewMessage(domain.DirectionInbound, "hello", "telegram")); err != nil {
		t.Fatalf("ForwardInbound should swallow analysis errors: %v", err)
	}
	if len(outbox.created) != 1 || outbox.created[0].Event != nil || outbox.created[0].RawLLMOutput != "not-json" {
		t.Fatalf("unexpected outbox item: %+v", outbox.created)
	}
	if outbox.created[0].Status != domain.ACPOutboxStatusFailed || outbox.created[0].ErrorMessage == "" {
		t.Fatalf("outbox item not created as failed: %+v", outbox.created[0])
	}
	if outbox.failedID != "" {
		t.Fatalf("malformed analysis should not mark dispatch failure: %+v", outbox)
	}
	if len(dispatcher.events) != 0 {
		t.Fatalf("dispatcher should not be called: %+v", dispatcher.events)
	}
}

func TestACPForwarderMarksDecisionSkip(t *testing.T) {
	event := testACPEvent()
	event.Routing.Confidence = 0.2
	outbox := &fakeACPOutbox{}
	dispatcher := &fakeACPDispatcher{}
	forwarder := NewACPForwarder(ACPForwarderDeps{
		Analyzer:   &fakeACPAnalyzer{result: ACPAnalysisResult{Event: event, RawOutput: `{"ok":true}`}},
		Outbox:     outbox,
		Dispatcher: dispatcher,
		Config:     ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp", MinConfidence: 0.8},
	})

	if err := forwarder.ForwardInbound(context.Background(), *domain.NewMessage(domain.DirectionInbound, "hello", "telegram")); err != nil {
		t.Fatalf("ForwardInbound: %v", err)
	}
	if outbox.skippedID != outbox.created[0].ID || outbox.skipReason != "low_confidence" {
		t.Fatalf("outbox not marked skipped: %+v", outbox)
	}
	if len(dispatcher.events) != 0 {
		t.Fatalf("dispatcher should not be called: %+v", dispatcher.events)
	}
}

func TestACPForwarderMarksDispatchFailure(t *testing.T) {
	event := testACPEvent()
	outbox := &fakeACPOutbox{}
	forwarder := NewACPForwarder(ACPForwarderDeps{
		Analyzer:   &fakeACPAnalyzer{result: ACPAnalysisResult{Event: event, RawOutput: `{"ok":true}`}},
		Outbox:     outbox,
		Dispatcher: &fakeACPDispatcher{result: ACPDispatchResult{StatusCode: 502}, err: errors.New("bad gateway")},
		Config:     ACPForwardConfig{Enabled: true, EndpointURL: "https://example.test/acp", MinConfidence: 0.8},
	})

	if err := forwarder.ForwardInbound(context.Background(), *domain.NewMessage(domain.DirectionInbound, "hello", "telegram")); err != nil {
		t.Fatalf("ForwardInbound should swallow dispatch errors: %v", err)
	}
	if outbox.failedID != outbox.created[0].ID || outbox.failedStatus == nil || *outbox.failedStatus != 502 || outbox.failedMessage == "" {
		t.Fatalf("outbox not marked failed: %+v", outbox)
	}
}

type fakeACPAnalyzer struct {
	result ACPAnalysisResult
	err    error
}

func (a *fakeACPAnalyzer) Analyze(context.Context, domain.Message) (ACPAnalysisResult, error) {
	return a.result, a.err
}

type fakeACPOutbox struct {
	created          []*domain.ACPOutboxItem
	skippedID        string
	skipReason       string
	failedID         string
	failedMessage    string
	failedStatus     *int
	dispatchedID     string
	dispatchedStatus int
}

func (o *fakeACPOutbox) Create(_ context.Context, item *domain.ACPOutboxItem) error {
	item.Normalize()
	o.created = append(o.created, item)
	return nil
}

func (o *fakeACPOutbox) Get(context.Context, string) (*domain.ACPOutboxItem, error) {
	return nil, errors.New("not implemented")
}

func (o *fakeACPOutbox) MarkSkipped(_ context.Context, id string, reason string) error {
	o.skippedID = id
	o.skipReason = reason
	return nil
}

func (o *fakeACPOutbox) MarkFailed(_ context.Context, id string, message string, statusCode *int) error {
	o.failedID = id
	o.failedMessage = message
	o.failedStatus = statusCode
	return nil
}

func (o *fakeACPOutbox) MarkDispatched(_ context.Context, id string, statusCode int) error {
	o.dispatchedID = id
	o.dispatchedStatus = statusCode
	return nil
}

type fakeACPDispatcher struct {
	events []domain.ACPEvent
	result ACPDispatchResult
	err    error
}

func (d *fakeACPDispatcher) Dispatch(_ context.Context, event domain.ACPEvent) (ACPDispatchResult, error) {
	d.events = append(d.events, event)
	return d.result, d.err
}

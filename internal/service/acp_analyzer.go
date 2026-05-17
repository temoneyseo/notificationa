package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/user/notification-hub/internal/ai"
	"github.com/user/notification-hub/internal/domain"
)

const acpAnalyzerPrompt = `Analyze the inbound channel message and return only one JSON object.
Required fields:
- should_forward: boolean
- intent: short snake_case string
- project: optional safe identifier
- agent: optional safe identifier
- priority: one of low, normal, high
- confidence: number between 0 and 1
- summary: concise summary for an agent
- action: concrete next action for an agent
- entities: optional array of strings
- language: optional BCP-47 style language tag
- normalized_content: optional normalized version of the message

Do not include endpoint URLs, credentials, tokens, webhook secrets, or channel configuration.`

var acpSafeIdentifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

type ACPAnalyzerConfig struct {
	Processor      ai.Processor
	DefaultProject string
	DefaultAgent   string
}

type ACPAnalysisResult struct {
	Event     *domain.ACPEvent
	RawOutput string
}

type ACPAnalyzer struct {
	processor      ai.Processor
	defaultProject string
	defaultAgent   string
}

func NewACPAnalyzer(cfg ACPAnalyzerConfig) *ACPAnalyzer {
	processor := cfg.Processor
	if processor == nil {
		processor = ai.NoopProcessor{}
	}
	return &ACPAnalyzer{
		processor:      processor,
		defaultProject: cfg.DefaultProject,
		defaultAgent:   cfg.DefaultAgent,
	}
}

func (a *ACPAnalyzer) Analyze(ctx context.Context, msg domain.Message) (ACPAnalysisResult, error) {
	metadata := acpPromptMetadata(msg)
	content, err := acpPromptContent(msg, metadata)
	if err != nil {
		return ACPAnalysisResult{}, err
	}
	resp, err := a.processor.Process(ctx, ai.Request{
		Content:  content,
		Mode:     domain.AIProcessingCustom,
		Prompt:   acpAnalyzerPrompt,
		Metadata: metadata,
	})
	raw := strings.TrimSpace(resp.Content)
	result := ACPAnalysisResult{RawOutput: raw}
	if err != nil {
		return result, err
	}
	analysis, err := parseACPModelOutput(raw)
	if err != nil {
		return result, err
	}
	event, err := a.buildEvent(msg, analysis)
	if err != nil {
		return result, err
	}
	result.Event = event
	return result, nil
}

func acpPromptMetadata(msg domain.Message) map[string]any {
	return map[string]any{
		"platform":    msg.Source,
		"channel_id":  stringMetadata(msg.Metadata, "channel_id"),
		"author_id":   stringMetadata(msg.Metadata, "author_id"),
		"author_name": stringMetadata(msg.Metadata, "author_name"),
		"message_id":  msg.ID,
	}
}

type acpModelOutput struct {
	ShouldForward     *bool    `json:"should_forward"`
	Intent            string   `json:"intent"`
	Project           string   `json:"project"`
	Agent             string   `json:"agent"`
	Priority          string   `json:"priority"`
	Confidence        *float64 `json:"confidence"`
	Summary           string   `json:"summary"`
	Action            string   `json:"action"`
	Entities          []any    `json:"entities"`
	Language          string   `json:"language"`
	NormalizedContent string   `json:"normalized_content"`
}

func parseACPModelOutput(raw string) (acpModelOutput, error) {
	if raw == "" {
		return acpModelOutput{}, errors.New("empty acp analysis output")
	}
	var output acpModelOutput
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&output); err != nil {
		return acpModelOutput{}, fmt.Errorf("parse acp analysis json: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return acpModelOutput{}, errors.New("parse acp analysis json: unexpected trailing data")
	}
	if output.ShouldForward == nil {
		return acpModelOutput{}, errors.New("acp analysis missing should_forward")
	}
	if strings.TrimSpace(output.Intent) == "" {
		return acpModelOutput{}, errors.New("acp analysis missing intent")
	}
	if output.Confidence == nil || *output.Confidence < 0 || *output.Confidence > 1 {
		return acpModelOutput{}, errors.New("acp analysis confidence must be between 0 and 1")
	}
	if !validACPPriority(output.Priority) {
		return acpModelOutput{}, errors.New("acp analysis priority must be low, normal, or high")
	}
	if strings.TrimSpace(output.Summary) == "" {
		return acpModelOutput{}, errors.New("acp analysis missing summary")
	}
	if strings.TrimSpace(output.Action) == "" {
		return acpModelOutput{}, errors.New("acp analysis missing action")
	}
	return output, nil
}

func acpPromptContent(msg domain.Message, metadata map[string]any) (string, error) {
	payload := map[string]any{
		"content": msg.ContentOriginal,
	}
	for key, value := range metadata {
		payload[key] = value
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (a *ACPAnalyzer) buildEvent(msg domain.Message, output acpModelOutput) (*domain.ACPEvent, error) {
	confidence := 0.0
	if output.Confidence != nil {
		confidence = *output.Confidence
	}
	normalized := strings.TrimSpace(output.NormalizedContent)
	if normalized == "" {
		normalized = msg.ContentProcessed
	}
	if normalized == "" {
		normalized = msg.ContentOriginal
	}
	return &domain.ACPEvent{
		Version:   domain.ACPEventVersion,
		EventType: domain.ACPEventTypeChannelInboundAnalyzed,
		MessageID: msg.ID,
		Source: domain.ACPEventSource{
			Platform:   msg.Source,
			ChannelID:  stringMetadata(msg.Metadata, "channel_id"),
			AuthorID:   stringMetadata(msg.Metadata, "author_id"),
			AuthorName: stringMetadata(msg.Metadata, "author_name"),
		},
		Routing: domain.ACPRouting{
			ShouldForward: *output.ShouldForward,
			Project:       sanitizeACPIdentifier(output.Project, a.defaultProject),
			Agent:         sanitizeACPIdentifier(output.Agent, a.defaultAgent),
			Priority:      output.Priority,
			Confidence:    confidence,
		},
		Analysis: domain.ACPAnalysis{
			Intent:   sanitizeACPText(output.Intent, 96),
			Summary:  sanitizeACPText(output.Summary, 1024),
			Action:   sanitizeACPText(output.Action, 1024),
			Entities: sanitizeACPStringArray(output.Entities, 20, 128),
			Language: sanitizeACPText(output.Language, 32),
		},
		Content: domain.ACPContent{
			Original:   msg.ContentOriginal,
			Normalized: sanitizeACPText(normalized, 4096),
		},
	}, nil
}

func validACPPriority(priority string) bool {
	switch priority {
	case string(domain.PriorityLow), string(domain.PriorityNormal), string(domain.PriorityHigh):
		return true
	default:
		return false
	}
}

func sanitizeACPIdentifier(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" && acpSafeIdentifier.MatchString(value) {
		return value
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" && acpSafeIdentifier.MatchString(fallback) {
		return fallback
	}
	return "default"
}

func sanitizeACPStringArray(values []any, maxItems int, maxLen int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if len(out) >= maxItems {
			break
		}
		text, ok := value.(string)
		if !ok {
			continue
		}
		text = sanitizeACPText(text, maxLen)
		if text != "" {
			out = append(out, text)
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func sanitizeACPText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}

func stringMetadata(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if ok {
		return text
	}
	return fmt.Sprint(value)
}

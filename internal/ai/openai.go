package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/domain"
)

type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

type OpenAIProcessor struct {
	client  *http.Client
	apiKey  string
	model   string
	baseURL string
}

func NewOpenAIProcessor(cfg OpenAIConfig) *OpenAIProcessor {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &OpenAIProcessor{
		client:  &http.Client{Timeout: cfg.Timeout},
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
	}
}

func (p *OpenAIProcessor) Process(ctx context.Context, req Request) (Response, error) {
	if p.apiKey == "" {
		return Response{}, errors.New("openai api key is not configured")
	}
	prompt := buildPrompt(req)
	payload := map[string]any{
		"model": p.model,
		"messages": []map[string]string{
			{"role": "system", "content": prompt},
			{"role": "user", "content": req.Content},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("openai returned status %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Response{}, err
	}
	if len(parsed.Choices) == 0 {
		return Response{}, errors.New("openai returned no choices")
	}
	return Response{Content: strings.TrimSpace(parsed.Choices[0].Message.Content)}, nil
}

func buildPrompt(req Request) string {
	if req.Mode == domain.AIProcessingCustom && req.Prompt != "" {
		return req.Prompt
	}
	switch req.Mode {
	case domain.AIProcessingSummarize:
		return "Summarize the notification clearly and concisely. Preserve important entities, numbers, urgency, and action items."
	case domain.AIProcessingTranslate:
		return "Translate the notification into concise Chinese while preserving technical terms, numbers, urgency, and action items."
	case domain.AIProcessingCustom:
		if req.Prompt != "" {
			return req.Prompt
		}
	}
	return "Return the notification content unchanged."
}

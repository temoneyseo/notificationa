package ai

import (
	"context"

	"github.com/user/notification-hub/internal/domain"
)

type Request struct {
	Content  string
	Mode     domain.AIProcessing
	Prompt   string
	Metadata map[string]any
}

type Response struct {
	Content  string
	Fallback bool
}

type Processor interface {
	Process(ctx context.Context, req Request) (Response, error)
}

type NoopProcessor struct{}

func (NoopProcessor) Process(_ context.Context, req Request) (Response, error) {
	return Response{Content: req.Content}, nil
}

type SafeProcessor struct {
	Processor Processor
}

func (p SafeProcessor) Process(ctx context.Context, req Request) (Response, error) {
	processor := p.Processor
	if processor == nil {
		processor = NoopProcessor{}
	}
	if req.Mode == "" || req.Mode == domain.AIProcessingNone {
		return Response{Content: req.Content}, nil
	}
	resp, err := processor.Process(ctx, req)
	if err != nil {
		return Response{Content: req.Content, Fallback: true}, nil
	}
	if resp.Content == "" {
		resp.Content = req.Content
		resp.Fallback = true
	}
	return resp, nil
}

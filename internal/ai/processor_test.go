package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func TestSafeProcessorFallsBackToOriginalOnError(t *testing.T) {
	processor := SafeProcessor{Processor: processorFunc(func(context.Context, Request) (Response, error) {
		return Response{}, errors.New("provider down")
	})}

	resp, err := processor.Process(context.Background(), Request{
		Content: "raw message",
		Mode:    domain.AIProcessingSummarize,
	})
	if err != nil {
		t.Fatalf("Process should not return provider error: %v", err)
	}
	if resp.Content != "raw message" || !resp.Fallback {
		t.Fatalf("response = %+v", resp)
	}
}

type processorFunc func(context.Context, Request) (Response, error)

func (f processorFunc) Process(ctx context.Context, req Request) (Response, error) {
	return f(ctx, req)
}

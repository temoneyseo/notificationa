package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/user/notification-hub/internal/domain"
)

type ACPHTTPClientConfig struct {
	EndpointURL string
	AuthToken   string
	HTTPClient  *http.Client
}

type ACPHTTPClient struct {
	endpointURL string
	authToken   string
	httpClient  *http.Client
}

func NewACPHTTPClient(cfg ACPHTTPClientConfig) *ACPHTTPClient {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &ACPHTTPClient{
		endpointURL: cfg.EndpointURL,
		authToken:   cfg.AuthToken,
		httpClient:  client,
	}
}

func (c *ACPHTTPClient) Send(ctx context.Context, event domain.ACPEvent) (ACPDispatchResult, error) {
	if c.endpointURL == "" {
		return ACPDispatchResult{}, fmt.Errorf("acp endpoint url is required")
	}
	body, err := json.Marshal(event)
	if err != nil {
		return ACPDispatchResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL, bytes.NewReader(body))
	if err != nil {
		return ACPDispatchResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ACPDispatchResult{}, err
	}
	defer resp.Body.Close()
	result := ACPDispatchResult{StatusCode: resp.StatusCode}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(readACPErrorBody(resp.Body, 1024))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return result, fmt.Errorf("acp endpoint returned status %d: %s", resp.StatusCode, message)
	}
	return result, nil
}

func readACPErrorBody(r io.Reader, limit int64) string {
	data, err := io.ReadAll(io.LimitReader(r, limit))
	if err != nil {
		return ""
	}
	return string(data)
}

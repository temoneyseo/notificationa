package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/user/notification-hub/internal/domain"
	"github.com/user/notification-hub/internal/security"
)

type WebhookDispatcherDeps struct {
	Webhooks   WebhookStore
	HTTPClient *http.Client
	Cipher     Cipher
}

// Cipher decrypts encrypted secrets before use.
type Cipher interface {
	DecryptString(ciphertext string) (string, error)
}

type HTTPWebhookDispatcher struct {
	deps WebhookDispatcherDeps
}

func NewWebhookDispatcher(deps WebhookDispatcherDeps) *HTTPWebhookDispatcher {
	if deps.HTTPClient == nil {
		deps.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPWebhookDispatcher{deps: deps}
}

func (d *HTTPWebhookDispatcher) Dispatch(ctx context.Context, event string, msg domain.Message) error {
	hooks, err := d.deps.Webhooks.ListActive(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, hook := range hooks {
		if !eventMatches(hook.Events, event) {
			continue
		}
		payload := map[string]any{
			"event":   event,
			"message": msg,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, hook.URL, bytes.NewReader(body))
		if err != nil {
			cancel()
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if hook.Secret != "" {
			secret := hook.Secret
			if d.deps.Cipher != nil {
				if decrypted, err := d.deps.Cipher.DecryptString(secret); err == nil {
					secret = decrypted
				}
			}
			req.Header.Set(security.SignatureHeader, security.SignHMACSHA256(secret, body))
		}
		resp, err := d.deps.HTTPClient.Do(req)
		cancel()
		if err != nil {
			_ = d.deps.Webhooks.MarkFailed(ctx, hook.ID, err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			err := fmt.Errorf("webhook %s returned status %d", hook.ID, resp.StatusCode)
			_ = d.deps.Webhooks.MarkFailed(ctx, hook.ID, err.Error())
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = d.deps.Webhooks.MarkTriggered(ctx, hook.ID)
	}
	return firstErr
}

func eventMatches(events []string, event string) bool {
	for _, item := range events {
		if item == event || item == "*" {
			return true
		}
	}
	return false
}

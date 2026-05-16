package handlers

import (
	"net/http"
	"strings"
	"testing"
)

func TestChannelListMasksConfigChannels(t *testing.T) {
	env := newHandlerTestEnv(t)
	resp := performJSON(env.router, http.MethodGet, "/api/v1/channels", nil)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), "telegram-token") {
		t.Fatalf("response leaked token: %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "********") {
		t.Fatalf("response did not contain masked config: %s", resp.Body.String())
	}
}

func TestChannelMutationsReturnConfigOnly(t *testing.T) {
	env := newHandlerTestEnv(t)

	for _, tc := range []struct {
		method string
		path   string
		body   map[string]any
	}{
		{method: http.MethodPost, path: "/api/v1/channels", body: map[string]any{"platform": "telegram", "name": "x"}},
		{method: http.MethodPut, path: "/api/v1/channels/x", body: map[string]any{"platform": "telegram", "name": "x"}},
		{method: http.MethodDelete, path: "/api/v1/channels/x"},
	} {
		resp := performJSON(env.router, tc.method, tc.path, tc.body)
		if resp.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s status = %d body=%s", tc.method, tc.path, resp.Code, resp.Body.String())
		}
		if !strings.Contains(resp.Body.String(), "channels are managed by config") {
			t.Fatalf("%s %s body=%s", tc.method, tc.path, resp.Body.String())
		}
	}
}

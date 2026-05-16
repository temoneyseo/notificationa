package handlers

import (
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	env := newHandlerTestEnv(t)
	resp := performJSON(env.router, http.MethodGet, "/health", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/user/notification-hub/internal/domain"
)

func TestACPHTTPClientPostsEventJSONWithHeaders(t *testing.T) {
	event := testACPEvent()
	var gotMethod, gotContentType, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("Decode body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewACPHTTPClient(ACPHTTPClientConfig{
		EndpointURL: server.URL,
		AuthToken:   "secret-token",
		HTTPClient:  server.Client(),
	})
	result, err := client.Send(context.Background(), *event)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("status code = %d", result.StatusCode)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content type = %q", gotContentType)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if gotBody["version"] != domain.ACPEventVersion || gotBody["event_type"] != domain.ACPEventTypeChannelInboundAnalyzed {
		t.Fatalf("unexpected body: %+v", gotBody)
	}
	bodyJSON, _ := json.Marshal(gotBody)
	if strings.Contains(string(bodyJSON), "secret-token") {
		t.Fatalf("request body leaked auth token: %s", bodyJSON)
	}
}

func TestACPHTTPClientTreatsNon2xxAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewACPHTTPClient(ACPHTTPClientConfig{EndpointURL: server.URL, HTTPClient: server.Client()})
	result, err := client.Send(context.Background(), *testACPEvent())
	if err == nil {
		t.Fatal("Send should fail for non-2xx")
	}
	if result.StatusCode != http.StatusBadGateway {
		t.Fatalf("status code = %d", result.StatusCode)
	}
}

func TestACPHTTPClientReturnsNetworkFailure(t *testing.T) {
	client := NewACPHTTPClient(ACPHTTPClientConfig{EndpointURL: "http://127.0.0.1:1/acp"})
	if _, err := client.Send(context.Background(), *testACPEvent()); err == nil {
		t.Fatal("Send should fail for network error")
	}
}

func testACPEvent() *domain.ACPEvent {
	return &domain.ACPEvent{
		Version:   domain.ACPEventVersion,
		EventType: domain.ACPEventTypeChannelInboundAnalyzed,
		MessageID: "msg_1",
		Source:    domain.ACPEventSource{Platform: "telegram", ChannelID: "-100", AuthorID: "u1", AuthorName: "alice"},
		Routing:   domain.ACPRouting{ShouldForward: true, Project: "notification", Agent: "triage", Priority: "normal", Confidence: 0.9},
		Analysis:  domain.ACPAnalysis{Intent: "docs_request", Summary: "summary", Action: "action", Entities: []string{"README"}, Language: "en"},
		Content:   domain.ACPContent{Original: "hello", Normalized: "hello"},
	}
}

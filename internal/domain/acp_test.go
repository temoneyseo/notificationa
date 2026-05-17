package domain

import (
	"encoding/json"
	"testing"
)

func TestACPEventStableWireJSON(t *testing.T) {
	event := ACPEvent{
		Version:   ACPEventVersion,
		EventType: ACPEventTypeChannelInboundAnalyzed,
		MessageID: "msg_123",
		Source: ACPEventSource{
			Platform:   "telegram",
			ChannelID:  "-100",
			AuthorID:   "u1",
			AuthorName: "alice",
		},
		Routing: ACPRouting{
			ShouldForward: true,
			Project:       "notification",
			Agent:         "triage",
			Priority:      "normal",
			Confidence:    0.86,
		},
		Analysis: ACPAnalysis{
			Intent:   "docs_request",
			Summary:  "README lacks quick API send examples.",
			Action:   "Update documentation with curl examples.",
			Entities: []string{"README", "API", "curl"},
			Language: "en",
		},
		Content: ACPContent{
			Original:   "readme missing api send examples",
			Normalized: "README is missing quick API send examples.",
		},
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"version", "event_type", "message_id", "source", "routing", "analysis", "content"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("event JSON missing %q: %s", key, data)
		}
	}
	if got["version"] != "2026-05-17" {
		t.Fatalf("version = %q", got["version"])
	}
	if got["event_type"] != "channel.inbound.analyzed" {
		t.Fatalf("event_type = %q", got["event_type"])
	}
}

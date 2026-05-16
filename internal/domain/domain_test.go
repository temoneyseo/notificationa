package domain

import "testing"

func TestNewMessageDefaults(t *testing.T) {
	msg := NewMessage(DirectionOutbound, "content", "api")
	if msg.ID == "" {
		t.Fatal("message ID should be generated")
	}
	if msg.Status != StatusPending {
		t.Fatalf("status = %s", msg.Status)
	}
	if msg.Priority != PriorityNormal {
		t.Fatalf("priority = %s", msg.Priority)
	}
	if msg.ContentProcessed != "content" {
		t.Fatalf("processed content = %q", msg.ContentProcessed)
	}
}

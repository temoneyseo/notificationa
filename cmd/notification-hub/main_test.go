package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureLogOutputWithEmptyPathKeepsDefaultWriter(t *testing.T) {
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	var buf bytes.Buffer
	log.SetOutput(&buf)

	writer, cleanup, err := configureLogOutput("")
	if err != nil {
		t.Fatalf("configureLogOutput returned error: %v", err)
	}
	defer cleanup()

	if writer != &buf {
		t.Fatalf("writer = %T, want current log writer", writer)
	}
	log.Print("default log target")
	if !strings.Contains(buf.String(), "default log target") {
		t.Fatalf("default log output = %q, want message", buf.String())
	}
}

func TestConfigureLogOutputWithPathAppendsToFile(t *testing.T) {
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	logPath := filepath.Join(t.TempDir(), "notification-hub.log")
	if err := os.WriteFile(logPath, []byte("existing\n"), 0644); err != nil {
		t.Fatalf("write existing log: %v", err)
	}

	writer, cleanup, err := configureLogOutput(logPath)
	if err != nil {
		t.Fatalf("configureLogOutput returned error: %v", err)
	}
	defer cleanup()

	log.Print("new log line")

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(content), "existing\n") {
		t.Fatalf("log file content = %q, want existing content preserved", string(content))
	}
	if !strings.Contains(string(content), "new log line") {
		t.Fatalf("log file content = %q, want new log line", string(content))
	}
	if writer == nil {
		t.Fatal("writer is nil")
	}
}

func TestConfigureLogOutputReturnsErrorForMissingDirectory(t *testing.T) {
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()
	t.Cleanup(func() {
		log.SetOutput(originalWriter)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	})

	missingPath := filepath.Join(t.TempDir(), "missing", "notification-hub.log")

	_, cleanup, err := configureLogOutput(missingPath)
	if err == nil {
		cleanup()
		t.Fatal("configureLogOutput returned nil error, want failure for missing directory")
	}
	if !strings.Contains(err.Error(), "open log file") {
		t.Fatalf("error = %v, want open log file context", err)
	}
}

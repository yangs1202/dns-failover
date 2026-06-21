package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSlackNotifierSendsWebhook(t *testing.T) {
	t.Parallel()

	var gotPayload slackPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("expected application/json, got %q", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	notifier, err := NewSlackNotifier(server.URL)
	if err != nil {
		t.Fatalf("NewSlackNotifier returned error: %v", err)
	}

	err = notifier.Notify(context.Background(), Event{
		Title: "dns-failover updated",
		Fields: map[string]string{
			"leader_region": "gs",
			"active_region": "sg",
			"target_name":   "sg.example.invalid",
		},
	})
	if err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}

	if gotPayload.Text == "" {
		t.Fatal("expected Slack text payload")
	}
	if want := "active_region: sg"; !strings.Contains(gotPayload.Text, want) {
		t.Fatalf("expected payload to contain %q, got %q", want, gotPayload.Text)
	}
}

func TestNoopNotifierDoesNothing(t *testing.T) {
	t.Parallel()

	if err := (NoopNotifier{}).Notify(context.Background(), Event{Title: "ignored"}); err != nil {
		t.Fatalf("Notify returned error: %v", err)
	}
}

func TestNewSlackNotifierRejectsEmptyURL(t *testing.T) {
	t.Parallel()

	_, err := NewSlackNotifier(" ")
	if err == nil {
		t.Fatal("expected empty webhook error")
	}
}

func TestSlackNotifierReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	notifier, err := NewSlackNotifier(server.URL)
	if err != nil {
		t.Fatalf("NewSlackNotifier returned error: %v", err)
	}

	err = notifier.Notify(context.Background(), Event{Title: "failed"})
	if err == nil {
		t.Fatal("expected webhook error")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestFormatSlackTextIncludesOrderedAndCustomFields(t *testing.T) {
	t.Parallel()

	text := formatSlackText(Event{
		Title: "title",
		Fields: map[string]string{
			"target_name": "target.example.invalid",
			"custom":      "value",
			"empty":       "",
		},
	})

	if !strings.Contains(text, "target_name: target.example.invalid") {
		t.Fatalf("expected ordered target_name field, got %q", text)
	}
	if !strings.Contains(text, "custom: value") {
		t.Fatalf("expected custom field, got %q", text)
	}
	if strings.Contains(text, "empty") {
		t.Fatalf("expected empty field to be skipped, got %q", text)
	}
	if !contains([]string{"a", "b"}, "b") {
		t.Fatal("expected contains to find value")
	}
	if contains([]string{"a", "b"}, "c") {
		t.Fatal("expected contains to miss value")
	}
}

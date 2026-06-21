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

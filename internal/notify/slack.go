package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Notifier interface {
	Notify(ctx context.Context, event Event) error
}

type Event struct {
	Title  string
	Fields map[string]string
}

type NoopNotifier struct{}

func (NoopNotifier) Notify(context.Context, Event) error {
	return nil
}

type SlackNotifier struct {
	webhookURL string
	client     *http.Client
}

func NewSlackNotifier(webhookURL string) (SlackNotifier, error) {
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		return SlackNotifier{}, fmt.Errorf("slack webhook url is required")
	}

	return SlackNotifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}, nil
}

func (n SlackNotifier) Notify(ctx context.Context, event Event) error {
	payload := slackPayload{
		Text: formatSlackText(event),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform slack request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("read slack response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook failed: status=%d body=%s", resp.StatusCode, string(responseBody))
	}

	return nil
}

type slackPayload struct {
	Text string `json:"text"`
}

func formatSlackText(event Event) string {
	var builder strings.Builder
	builder.WriteString(event.Title)

	orderedKeys := []string{"leader_region", "active_region", "target_name", "record_name", "error"}
	for _, key := range orderedKeys {
		value, ok := event.Fields[key]
		if !ok || value == "" {
			continue
		}
		builder.WriteString("\n")
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(value)
	}

	for key, value := range event.Fields {
		if value == "" || contains(orderedKeys, key) {
			continue
		}
		builder.WriteString("\n")
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(value)
	}

	return builder.String()
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

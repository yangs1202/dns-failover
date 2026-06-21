package dnsprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const dnsimpleAPIBaseURL = "https://api.dnsimple.com/v2"

type DNSimpleProvider struct {
	apiToken   string
	accountID  string
	zoneName   string
	recordID   string
	recordType string
	ttl        int
	baseURL    string
	client     *http.Client
}

func NewDNSimpleProvider(cfg Config) (Provider, error) {
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, fmt.Errorf("dnsimple api token is required")
	}
	if strings.TrimSpace(cfg.AccountID) == "" {
		return nil, fmt.Errorf("dnsimple account id is required")
	}
	if strings.TrimSpace(cfg.ZoneID) == "" {
		return nil, fmt.Errorf("dnsimple zone name is required")
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		return nil, fmt.Errorf("dnsimple record id is required")
	}

	recordType := strings.TrimSpace(cfg.RecordType)
	if recordType == "" {
		recordType = "CNAME"
	}
	if recordType != "CNAME" {
		return nil, fmt.Errorf("dnsimple provider only supports CNAME records, got %q", recordType)
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 60
	}

	return DNSimpleProvider{
		apiToken:   cfg.APIToken,
		accountID:  strings.TrimSpace(cfg.AccountID),
		zoneName:   strings.TrimSuffix(strings.TrimSpace(cfg.ZoneID), "."),
		recordID:   strings.TrimSpace(cfg.RecordID),
		recordType: recordType,
		ttl:        ttl,
		baseURL:    dnsimpleAPIBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (p DNSimpleProvider) UpdateCNAME(ctx context.Context, change CNAMEChange) error {
	zoneName := p.zoneName
	if change.ZoneID != "" {
		zoneName = strings.TrimSuffix(strings.TrimSpace(change.ZoneID), ".")
	}
	recordID := p.recordID
	if change.RecordID != "" {
		recordID = strings.TrimSpace(change.RecordID)
	}

	payload := dnsimpleRecordRequest{
		Content: ensureTrailingDot(change.TargetName),
		TTL:     p.ttl,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal dnsimple dns request: %w", err)
	}

	endpoint := fmt.Sprintf(
		"%s/%s/zones/%s/records/%s",
		strings.TrimRight(p.baseURL, "/"),
		url.PathEscape(p.accountID),
		url.PathEscape(zoneName),
		url.PathEscape(recordID),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create dnsimple dns request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform dnsimple dns request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read dnsimple dns response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("dnsimple dns request failed: status=%d body=%s", resp.StatusCode, string(responseBody))
	}

	return nil
}

type dnsimpleRecordRequest struct {
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
}

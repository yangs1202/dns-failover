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

const digitalOceanAPIBaseURL = "https://api.digitalocean.com/v2"

type DigitalOceanProvider struct {
	apiToken   string
	domainName string
	recordID   string
	recordName string
	recordType string
	ttl        int
	baseURL    string
	client     *http.Client
}

func NewDigitalOceanProvider(cfg Config) (Provider, error) {
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, fmt.Errorf("digitalocean api token is required")
	}
	if strings.TrimSpace(cfg.ZoneID) == "" {
		return nil, fmt.Errorf("digitalocean domain name is required")
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		return nil, fmt.Errorf("digitalocean record id is required")
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return nil, fmt.Errorf("digitalocean record name is required")
	}

	recordType := strings.TrimSpace(cfg.RecordType)
	if recordType == "" {
		recordType = "CNAME"
	}
	if recordType != "CNAME" {
		return nil, fmt.Errorf("digitalocean provider only supports CNAME records, got %q", recordType)
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 60
	}

	domainName := strings.TrimSuffix(strings.TrimSpace(cfg.ZoneID), ".")
	recordName := relativeDNSName(cfg.RecordName, domainName)

	return DigitalOceanProvider{
		apiToken:   cfg.APIToken,
		domainName: domainName,
		recordID:   strings.TrimSpace(cfg.RecordID),
		recordName: recordName,
		recordType: recordType,
		ttl:        ttl,
		baseURL:    digitalOceanAPIBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (p DigitalOceanProvider) UpdateCNAME(ctx context.Context, change CNAMEChange) error {
	domainName := p.domainName
	if change.ZoneID != "" {
		domainName = strings.TrimSuffix(strings.TrimSpace(change.ZoneID), ".")
	}
	recordID := p.recordID
	if change.RecordID != "" {
		recordID = strings.TrimSpace(change.RecordID)
	}
	recordName := p.recordName
	if change.RecordName != "" {
		recordName = relativeDNSName(change.RecordName, domainName)
	}

	payload := digitalOceanRecordRequest{
		Type: p.recordType,
		Name: recordName,
		Data: ensureTrailingDot(change.TargetName),
		TTL:  p.ttl,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal digitalocean dns request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/domains/%s/records/%s", strings.TrimRight(p.baseURL, "/"), url.PathEscape(domainName), url.PathEscape(recordID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create digitalocean dns request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform digitalocean dns request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read digitalocean dns response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("digitalocean dns request failed: status=%d body=%s", resp.StatusCode, string(responseBody))
	}

	return nil
}

type digitalOceanRecordRequest struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Data string `json:"data"`
	TTL  int    `json:"ttl"`
}

package dnsprovider

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

const cloudflareAPIBaseURL = "https://api.cloudflare.com/client/v4"

type CloudflareProvider struct {
	apiToken   string
	zoneID     string
	recordID   string
	recordName string
	recordType string
	baseURL    string
	client     *http.Client
}

func NewCloudflareProvider(cfg Config) (Provider, error) {
	if strings.TrimSpace(cfg.APIToken) == "" {
		return nil, fmt.Errorf("cloudflare api token is required")
	}
	if strings.TrimSpace(cfg.ZoneID) == "" {
		return nil, fmt.Errorf("cloudflare zone id is required")
	}
	if strings.TrimSpace(cfg.RecordID) == "" {
		return nil, fmt.Errorf("cloudflare record id is required")
	}
	if strings.TrimSpace(cfg.RecordName) == "" {
		return nil, fmt.Errorf("cloudflare record name is required")
	}

	recordType := strings.TrimSpace(cfg.RecordType)
	if recordType == "" {
		recordType = "CNAME"
	}
	if recordType != "CNAME" {
		return nil, fmt.Errorf("cloudflare provider only supports CNAME records, got %q", recordType)
	}

	return CloudflareProvider{
		apiToken:   cfg.APIToken,
		zoneID:     strings.TrimSpace(cfg.ZoneID),
		recordID:   strings.TrimSpace(cfg.RecordID),
		recordName: strings.TrimSuffix(strings.TrimSpace(cfg.RecordName), "."),
		recordType: recordType,
		baseURL:    cloudflareAPIBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (p CloudflareProvider) UpdateCNAME(ctx context.Context, change CNAMEChange) error {
	targetName := strings.TrimSuffix(strings.TrimSpace(change.TargetName), ".")
	if targetName == "" {
		return fmt.Errorf("target name is required")
	}

	zoneID := p.zoneID
	if change.ZoneID != "" {
		zoneID = change.ZoneID
	}
	recordID := p.recordID
	if change.RecordID != "" {
		recordID = change.RecordID
	}
	recordName := p.recordName
	if change.RecordName != "" {
		recordName = strings.TrimSuffix(strings.TrimSpace(change.RecordName), ".")
	}

	payload := cloudflareDNSRecordRequest{
		Type:    p.recordType,
		Name:    recordName,
		Content: targetName,
		TTL:     60,
		Proxied: false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal cloudflare dns request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/zones/%s/dns_records/%s", strings.TrimRight(p.baseURL, "/"), zoneID, recordID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create cloudflare dns request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform cloudflare dns request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read cloudflare dns response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cloudflare dns request failed: status=%d body=%s", resp.StatusCode, string(responseBody))
	}

	var cfResp cloudflareResponse
	if err := json.Unmarshal(responseBody, &cfResp); err != nil {
		return fmt.Errorf("decode cloudflare dns response: %w", err)
	}
	if !cfResp.Success {
		return fmt.Errorf("cloudflare dns request was not successful: errors=%v", cfResp.Errors)
	}

	return nil
}

type cloudflareDNSRecordRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type cloudflareResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

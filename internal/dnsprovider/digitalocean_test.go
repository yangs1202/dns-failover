package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDigitalOceanProviderUpdatesCNAME(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotPayload digitalOceanRecordRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/domains/example.invalid/records/record-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"domain_record":{"id":1}}`))
	}))
	defer server.Close()

	provider, err := NewDigitalOceanProvider(Config{
		APIToken:   "token",
		ZoneID:     "example.invalid",
		RecordID:   "record-1",
		RecordName: "vip.example.invalid",
		RecordType: "CNAME",
		TTL:        30,
	})
	if err != nil {
		t.Fatalf("NewDigitalOceanProvider returned error: %v", err)
	}

	doProvider := provider.(DigitalOceanProvider)
	doProvider.baseURL = server.URL
	if err := doProvider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: "region.example.invalid"}); err != nil {
		t.Fatalf("UpdateCNAME returned error: %v", err)
	}

	if gotAuth != "Bearer token" {
		t.Fatalf("unexpected Authorization header %q", gotAuth)
	}
	if gotPayload.Name != "vip" {
		t.Fatalf("expected relative record name vip, got %q", gotPayload.Name)
	}
	if gotPayload.Data != "region.example.invalid." {
		t.Fatalf("expected target data, got %q", gotPayload.Data)
	}
	if gotPayload.TTL != 30 {
		t.Fatalf("expected ttl 30, got %d", gotPayload.TTL)
	}
}

func TestNewDigitalOceanProviderRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []Config{
		{},
		{APIToken: "token"},
		{APIToken: "token", ZoneID: "example.invalid"},
		{APIToken: "token", ZoneID: "example.invalid", RecordID: "record-1"},
		{APIToken: "token", ZoneID: "example.invalid", RecordID: "record-1", RecordName: "vip.example.invalid", RecordType: "A"},
	}
	for _, cfg := range tests {
		if _, err := NewDigitalOceanProvider(cfg); err == nil {
			t.Fatalf("expected config error for %+v", cfg)
		}
	}
}

func TestDigitalOceanProviderAppliesChangeOverrides(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotPayload digitalOceanRecordRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := DigitalOceanProvider{
		apiToken:   "token",
		domainName: "example.invalid",
		recordID:   "record-1",
		recordName: "vip",
		recordType: "CNAME",
		ttl:        60,
		baseURL:    server.URL,
		client:     server.Client(),
	}
	err := provider.UpdateCNAME(context.Background(), CNAMEChange{
		ZoneID:     "override.invalid",
		RecordID:   "record-2",
		RecordName: "vip.override.invalid",
		TargetName: "target.override.invalid",
	})
	if err != nil {
		t.Fatalf("UpdateCNAME returned error: %v", err)
	}
	if gotPath != "/domains/override.invalid/records/record-2" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotPayload.Name != "vip" {
		t.Fatalf("expected relative override name vip, got %q", gotPayload.Name)
	}
}

func TestDigitalOceanProviderReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	provider := DigitalOceanProvider{
		apiToken:   "token",
		domainName: "example.invalid",
		recordID:   "record-1",
		recordName: "vip",
		recordType: "CNAME",
		ttl:        60,
		baseURL:    server.URL,
		client:     server.Client(),
	}
	err := provider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: "target.example.invalid"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

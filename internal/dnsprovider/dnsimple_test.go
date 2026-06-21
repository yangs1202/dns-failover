package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDNSimpleProviderUpdatesCNAME(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotPayload dnsimpleRecordRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/123/zones/example.invalid/records/record-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"id":1}}`))
	}))
	defer server.Close()

	provider, err := NewDNSimpleProvider(Config{
		APIToken:   "token",
		AccountID:  "123",
		ZoneID:     "example.invalid",
		RecordID:   "record-1",
		RecordName: "vip.example.invalid",
		RecordType: "CNAME",
		TTL:        30,
	})
	if err != nil {
		t.Fatalf("NewDNSimpleProvider returned error: %v", err)
	}

	dnsimpleProvider := provider.(DNSimpleProvider)
	dnsimpleProvider.baseURL = server.URL
	if err := dnsimpleProvider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: "region.example.invalid"}); err != nil {
		t.Fatalf("UpdateCNAME returned error: %v", err)
	}

	if gotAuth != "Bearer token" {
		t.Fatalf("unexpected Authorization header %q", gotAuth)
	}
	if gotPayload.Content != "region.example.invalid." {
		t.Fatalf("expected target content, got %q", gotPayload.Content)
	}
	if gotPayload.TTL != 30 {
		t.Fatalf("expected ttl 30, got %d", gotPayload.TTL)
	}
}

func TestNewDNSimpleProviderRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []Config{
		{},
		{APIToken: "token"},
		{APIToken: "token", AccountID: "123"},
		{APIToken: "token", AccountID: "123", ZoneID: "example.invalid"},
		{APIToken: "token", AccountID: "123", ZoneID: "example.invalid", RecordID: "record-1", RecordType: "A"},
	}
	for _, cfg := range tests {
		if _, err := NewDNSimpleProvider(cfg); err == nil {
			t.Fatalf("expected config error for %+v", cfg)
		}
	}
}

func TestDNSimpleProviderAppliesChangeOverrides(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := DNSimpleProvider{
		apiToken:   "token",
		accountID:  "123",
		zoneName:   "example.invalid",
		recordID:   "record-1",
		recordType: "CNAME",
		ttl:        60,
		baseURL:    server.URL,
		client:     server.Client(),
	}
	err := provider.UpdateCNAME(context.Background(), CNAMEChange{
		ZoneID:     "override.invalid",
		RecordID:   "record-2",
		TargetName: "target.override.invalid",
	})
	if err != nil {
		t.Fatalf("UpdateCNAME returned error: %v", err)
	}
	if gotPath != "/123/zones/override.invalid/records/record-2" {
		t.Fatalf("unexpected path %q", gotPath)
	}
}

func TestDNSimpleProviderReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	provider := DNSimpleProvider{
		apiToken:   "token",
		accountID:  "123",
		zoneName:   "example.invalid",
		recordID:   "record-1",
		recordType: "CNAME",
		ttl:        60,
		baseURL:    server.URL,
		client:     server.Client(),
	}
	err := provider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: "target.example.invalid"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

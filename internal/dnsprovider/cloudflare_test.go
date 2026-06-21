package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewCloudflareProviderRequiresCredentials(t *testing.T) {
	t.Parallel()

	_, err := NewCloudflareProvider(Config{Name: "cloudflare"})
	if err == nil {
		t.Fatal("expected missing credential error")
	}
}

func TestCloudflareProviderUpdatesCNAME(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotPayload cloudflareDNSRecordRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Fatalf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/zones/zone-1/dns_records/record-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"errors":[]}`))
	}))
	defer server.Close()

	provider, err := NewCloudflareProvider(Config{
		APIToken:   "token",
		ZoneID:     "zone-1",
		RecordID:   "record-1",
		RecordName: "vip.example.invalid",
		RecordType: "CNAME",
		TTL:        1,
	})
	if err != nil {
		t.Fatalf("NewCloudflareProvider returned error: %v", err)
	}

	cfProvider := provider.(CloudflareProvider)
	cfProvider.baseURL = server.URL
	err = cfProvider.UpdateCNAME(context.Background(), CNAMEChange{
		TargetName: "gs.example.invalid.",
	})
	if err != nil {
		t.Fatalf("UpdateCNAME returned error: %v", err)
	}

	if gotAuth != "Bearer token" {
		t.Fatalf("unexpected Authorization header %q", gotAuth)
	}
	if gotPayload.Content != "gs.example.invalid" {
		t.Fatalf("expected target content, got %q", gotPayload.Content)
	}
	if gotPayload.Name != "vip.example.invalid" {
		t.Fatalf("expected record name, got %q", gotPayload.Name)
	}
	if gotPayload.Type != "CNAME" {
		t.Fatalf("expected CNAME, got %q", gotPayload.Type)
	}
	if gotPayload.TTL != 1 {
		t.Fatalf("expected ttl 1, got %d", gotPayload.TTL)
	}
}

func TestNewCloudflareProviderRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	tests := []Config{
		{},
		{APIToken: "token"},
		{APIToken: "token", ZoneID: "zone-1"},
		{APIToken: "token", ZoneID: "zone-1", RecordID: "record-1"},
		{APIToken: "token", ZoneID: "zone-1", RecordID: "record-1", RecordName: "vip.example.invalid", RecordType: "A"},
	}
	for _, cfg := range tests {
		if _, err := NewCloudflareProvider(cfg); err == nil {
			t.Fatalf("expected config error for %+v", cfg)
		}
	}
}

func TestCloudflareProviderReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"success":false}`))
	}))
	defer server.Close()

	provider := CloudflareProvider{
		apiToken:   "token",
		zoneID:     "zone-1",
		recordID:   "record-1",
		recordName: "vip.example.invalid",
		recordType: "CNAME",
		ttl:        60,
		baseURL:    server.URL,
		client:     server.Client(),
	}
	err := provider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: "target.example.invalid"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected status code in error, got %v", err)
	}
}

func TestCloudflareProviderReturnsDecodeAndUnsuccessfulErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "decode", body: `{`},
		{name: "unsuccessful", body: `{"success":false,"errors":[{"code":1000,"message":"bad"}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			provider := CloudflareProvider{
				apiToken:   "token",
				zoneID:     "zone-1",
				recordID:   "record-1",
				recordName: "vip.example.invalid",
				recordType: "CNAME",
				ttl:        60,
				baseURL:    server.URL,
				client:     server.Client(),
			}
			err := provider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: "target.example.invalid"})
			if err == nil {
				t.Fatal("expected response error")
			}
		})
	}
}

func TestCloudflareProviderRejectsEmptyTarget(t *testing.T) {
	t.Parallel()

	provider := CloudflareProvider{recordType: "CNAME"}
	err := provider.UpdateCNAME(context.Background(), CNAMEChange{TargetName: " "})
	if err == nil {
		t.Fatal("expected empty target error")
	}
}

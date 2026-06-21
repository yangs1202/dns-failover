package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
}

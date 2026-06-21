package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

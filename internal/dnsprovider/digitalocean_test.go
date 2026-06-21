package dnsprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

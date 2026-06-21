package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yangs1202/dns-failover/internal/config"
)

func TestHTTPCheckerTreats200AsHealthy(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewHTTPChecker(time.Second)
	result := checker.Check(context.Background(), config.Endpoint{RegionID: "region-a", URL: server.URL})

	if !result.Healthy {
		t.Fatalf("expected healthy result, got error %v", result.Err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 status, got %d", result.StatusCode)
	}
}

func TestHTTPCheckerTreatsNon200AsUnhealthy(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	checker := NewHTTPChecker(time.Second)
	result := checker.Check(context.Background(), config.Endpoint{RegionID: "region-a", URL: server.URL})

	if result.Healthy {
		t.Fatal("expected unhealthy result")
	}
	if result.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 status, got %d", result.StatusCode)
	}
}

func TestHTTPCheckerDoesNotFollowRedirects(t *testing.T) {
	t.Parallel()

	redirectTargetCalled := false
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectTargetCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer redirectTarget.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirectTarget.URL, http.StatusFound)
	}))
	defer server.Close()

	checker := NewHTTPChecker(time.Second)
	result := checker.Check(context.Background(), config.Endpoint{RegionID: "region-a", URL: server.URL})

	if result.Healthy {
		t.Fatal("expected redirect to be unhealthy")
	}
	if result.StatusCode != http.StatusFound {
		t.Fatalf("expected 302 status without following redirect, got %d", result.StatusCode)
	}
	if redirectTargetCalled {
		t.Fatal("expected redirect target not to be called")
	}
}

func TestHTTPCheckerTreatsRequestErrorAsUnhealthy(t *testing.T) {
	t.Parallel()

	checker := NewHTTPChecker(time.Millisecond)
	result := checker.Check(context.Background(), config.Endpoint{RegionID: "region-a", URL: ":// bad-url"})

	if result.Healthy {
		t.Fatal("expected unhealthy result")
	}
	if result.Err == nil {
		t.Fatal("expected request error")
	}
	if result.RegionID != "region-a" {
		t.Fatalf("expected region-a, got %q", result.RegionID)
	}
}

package config

import "testing"

func TestParseEndpoints(t *testing.T) {
	t.Parallel()

	endpoints, err := parseEndpoints("region-a=https://example-a.invalid/healthz,region-b=http://example-b.invalid/healthz")
	if err != nil {
		t.Fatalf("parseEndpoints returned error: %v", err)
	}

	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
	if endpoints[0].RegionID != "region-a" {
		t.Fatalf("expected first region region-a, got %q", endpoints[0].RegionID)
	}
}

func TestParseEndpointsRejectsDuplicateRegions(t *testing.T) {
	t.Parallel()

	_, err := parseEndpoints("region-a=https://example-a.invalid/healthz,region-a=https://example-b.invalid/healthz")
	if err == nil {
		t.Fatal("expected duplicate region error")
	}
}

func TestParseEndpointsRejectsUnsupportedScheme(t *testing.T) {
	t.Parallel()

	_, err := parseEndpoints("region-a=tcp://example-a.invalid:443")
	if err == nil {
		t.Fatal("expected unsupported scheme error")
	}
}

func TestParseDNSTargets(t *testing.T) {
	t.Parallel()

	targets, err := parseDNSTargets("region-a=region-a.example.invalid,region-b=region-b.example.invalid.")
	if err != nil {
		t.Fatalf("parseDNSTargets returned error: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[1].Name != "region-b.example.invalid" {
		t.Fatalf("expected trailing dot to be trimmed, got %q", targets[1].Name)
	}
}

func TestParseDNSTargetsRejectsURLs(t *testing.T) {
	t.Parallel()

	_, err := parseDNSTargets("region-a=https://region-a.example.invalid")
	if err == nil {
		t.Fatal("expected URL rejection error")
	}
}

func TestValidateRegionSetsRequiresMatchingRegions(t *testing.T) {
	t.Parallel()

	err := validateRegionSets(
		[]Endpoint{{RegionID: "region-a", URL: "https://example-a.invalid/healthz"}},
		[]DNSTarget{{RegionID: "region-b", Name: "region-b.example.invalid"}},
	)
	if err == nil {
		t.Fatal("expected mismatched region error")
	}
}

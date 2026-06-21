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

func TestParseEndpointsRejectsMalformedEntries(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"region-a",
		"=https://example-a.invalid/healthz",
		"region-a=",
		"region-a=%zz",
		"region-a=https:///healthz",
	}
	for _, raw := range tests {
		if _, err := parseEndpoints(raw); err == nil {
			t.Fatalf("expected parseEndpoints error for %q", raw)
		}
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

func TestParseDNSTargetsRejectsMalformedEntries(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"region-a",
		"=region-a.example.invalid",
		"region-a=",
		"region-a=region-a.example.invalid,region-a=region-b.example.invalid",
	}
	for _, raw := range tests {
		if _, err := parseDNSTargets(raw); err == nil {
			t.Fatalf("expected parseDNSTargets error for %q", raw)
		}
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

func TestValidateRegionSetsRequiresTargetsForAllEndpoints(t *testing.T) {
	t.Parallel()

	err := validateRegionSets(
		[]Endpoint{
			{RegionID: "region-a", URL: "https://example-a.invalid/healthz"},
			{RegionID: "region-b", URL: "https://example-b.invalid/healthz"},
		},
		[]DNSTarget{{RegionID: "region-a", Name: "region-a.example.invalid"}},
	)
	if err == nil {
		t.Fatal("expected missing DNS target error")
	}
}

func TestParseRegionPriority(t *testing.T) {
	t.Parallel()

	priority, err := parseRegionPriority("region-a,region-b,region-c")
	if err != nil {
		t.Fatalf("parseRegionPriority returned error: %v", err)
	}

	if len(priority) != 3 {
		t.Fatalf("expected 3 priority entries, got %d", len(priority))
	}
	if priority[0] != "region-a" {
		t.Fatalf("expected first priority region-a, got %q", priority[0])
	}
}

func TestParseRegionPriorityRejectsMalformedEntries(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"region-a,,region-b",
		"region-a,region-a",
	}
	for _, raw := range tests {
		if _, err := parseRegionPriority(raw); err == nil {
			t.Fatalf("expected parseRegionPriority error for %q", raw)
		}
	}
}

func TestValidateRegionPriorityRequiresAllRegions(t *testing.T) {
	t.Parallel()

	err := validateRegionPriority(
		[]string{"region-a"},
		[]Endpoint{
			{RegionID: "region-a", URL: "https://example-a.invalid/healthz"},
			{RegionID: "region-b", URL: "https://example-b.invalid/healthz"},
		},
	)
	if err == nil {
		t.Fatal("expected missing priority region error")
	}
}

func TestValidateRegionPriorityRejectsUnknownRegion(t *testing.T) {
	t.Parallel()

	err := validateRegionPriority(
		[]string{"region-a", "region-c"},
		[]Endpoint{
			{RegionID: "region-a", URL: "https://example-a.invalid/healthz"},
			{RegionID: "region-b", URL: "https://example-b.invalid/healthz"},
		},
	)
	if err == nil {
		t.Fatal("expected unknown priority region error")
	}
}

func TestParseDNSNames(t *testing.T) {
	t.Parallel()

	names, err := parseDNSNames("app.example.invalid.,api.example.invalid", "TEST_DNS_NAMES")
	if err != nil {
		t.Fatalf("parseDNSNames returned error: %v", err)
	}

	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
	if names[0] != "app.example.invalid" {
		t.Fatalf("expected trailing dot to be trimmed, got %q", names[0])
	}
}

func TestParseDNSNamesRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []string{
		"app.example.invalid,,api.example.invalid",
		"https://app.example.invalid",
		"app.example.invalid,app.example.invalid.",
	}
	for _, raw := range tests {
		if _, err := parseDNSNames(raw, "TEST_DNS_NAMES"); err == nil {
			t.Fatalf("expected parseDNSNames error for %q", raw)
		}
	}
}

func TestConfigSupportsThreeRegionCNAMEScenario(t *testing.T) {
	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/ncm-cgi/health,region-b=https://region-b.example.invalid/ncm-cgi/health,region-c=https://region-c.example.invalid/ncm-cgi/health")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid,region-b=region-b.example.invalid,region-c=region-c.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a,region-b,region-c")
	t.Setenv("DNS_FAILOVER_SERVICE_RECORDS", "app.example.invalid")
	t.Setenv("DNS_FAILOVER_CHECK_INTERVAL", "15s")
	t.Setenv("DNS_FAILOVER_ETCD_ENDPOINTS", "10.0.0.1:2379,10.0.0.2:2379,10.0.0.3:2379")
	t.Setenv("DNS_FAILOVER_ETCD_KEY_PREFIX", "/dns-failover-test")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "cloudflare")
	t.Setenv("DNS_FAILOVER_DNS_RECORD_NAME", "vip.example.invalid")
	t.Setenv("DNS_FAILOVER_DNS_RECORD_TYPE", "CNAME")
	t.Setenv("DNS_FAILOVER_DNS_TTL", "1")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}

	if cfg.DNSProvider.Provider != "cloudflare" {
		t.Fatalf("expected configured DNS provider, got %q", cfg.DNSProvider.Provider)
	}
	if cfg.DNSProvider.RecordName != "vip.example.invalid" {
		t.Fatalf("expected vip CNAME record, got %q", cfg.DNSProvider.RecordName)
	}
	if cfg.Endpoints[0].URL != "https://region-a.example.invalid/ncm-cgi/health" {
		t.Fatalf("expected configured health check path, got %q", cfg.Endpoints[0].URL)
	}
	if cfg.RegionPriority[0] != "region-a" {
		t.Fatalf("expected master priority region-a, got %q", cfg.RegionPriority[0])
	}
	if cfg.ServiceRecords[0] != "app.example.invalid" {
		t.Fatalf("expected service alias, got %q", cfg.ServiceRecords[0])
	}
	if cfg.CheckInterval.String() != "15s" {
		t.Fatalf("expected configured check interval, got %s", cfg.CheckInterval)
	}
	if cfg.DNSProvider.TTL != 1 {
		t.Fatalf("expected configured dns ttl 1, got %d", cfg.DNSProvider.TTL)
	}
	if len(cfg.Etcd.Endpoints) != 3 {
		t.Fatalf("expected three etcd endpoints, got %d", len(cfg.Etcd.Endpoints))
	}
	if cfg.Etcd.KeyPrefix != "/dns-failover-test/" {
		t.Fatalf("expected normalized etcd prefix, got %q", cfg.Etcd.KeyPrefix)
	}
}

func TestLoadFromEnvRequiresDNSProvider(t *testing.T) {
	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/healthz")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected missing DNS provider error")
	}
}

func TestLoadFromEnvDefaultsDNSRecordType(t *testing.T) {
	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/healthz")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "example")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.DNSProvider.RecordType != "CNAME" {
		t.Fatalf("expected default record type CNAME, got %q", cfg.DNSProvider.RecordType)
	}
	if cfg.CheckInterval.String() != "10s" {
		t.Fatalf("expected default check interval 10s, got %s", cfg.CheckInterval)
	}
	if cfg.Etcd.KeyPrefix != "/dns-failover/" {
		t.Fatalf("expected default etcd key prefix, got %q", cfg.Etcd.KeyPrefix)
	}
	if cfg.DNSProvider.TTL != 60 {
		t.Fatalf("expected default dns ttl 60, got %d", cfg.DNSProvider.TTL)
	}
}

func TestLoadFromEnvSupportsProviderSpecificDNSCredentials(t *testing.T) {
	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/healthz")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "route53")
	t.Setenv("DNS_FAILOVER_DNS_ACCOUNT_ID", "account-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "aws-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret-key")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv returned error: %v", err)
	}
	if cfg.DNSProvider.AccountID != "account-1" {
		t.Fatalf("expected provider account id, got %q", cfg.DNSProvider.AccountID)
	}
	if cfg.DNSProvider.AccessKeyID != "aws-access-key" {
		t.Fatalf("expected AWS access key fallback, got %q", cfg.DNSProvider.AccessKeyID)
	}
	if cfg.DNSProvider.SecretAccessKey != "aws-secret-key" {
		t.Fatalf("expected AWS secret key fallback, got %q", cfg.DNSProvider.SecretAccessKey)
	}
}

func TestLoadFromEnvRejectsInvalidDNSTTL(t *testing.T) {
	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/healthz")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "example")
	t.Setenv("DNS_FAILOVER_DNS_TTL", "0")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected invalid DNS TTL error")
	}
}

func TestLoadFromEnvRejectsInvalidDurationsAndLists(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{name: "dns ttl parse", env: map[string]string{"DNS_FAILOVER_DNS_TTL": "bad"}},
		{name: "health timeout parse", env: map[string]string{"DNS_FAILOVER_HEALTH_TIMEOUT": "bad"}},
		{name: "health timeout positive", env: map[string]string{"DNS_FAILOVER_HEALTH_TIMEOUT": "0s"}},
		{name: "check interval parse", env: map[string]string{"DNS_FAILOVER_CHECK_INTERVAL": "bad"}},
		{name: "check interval positive", env: map[string]string{"DNS_FAILOVER_CHECK_INTERVAL": "0s"}},
		{name: "etcd endpoint empty", env: map[string]string{"DNS_FAILOVER_ETCD_ENDPOINTS": "10.0.0.1:2379,"}},
		{name: "service record invalid", env: map[string]string{"DNS_FAILOVER_SERVICE_RECORDS": "https://app.example.invalid"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setMinimalEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			_, err := LoadFromEnv()
			if err == nil {
				t.Fatal("expected LoadFromEnv error")
			}
		})
	}
}

func TestLoadFromEnvRejectsInvalidEtcdKeyPrefix(t *testing.T) {
	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/healthz")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "example")
	t.Setenv("DNS_FAILOVER_ETCD_KEY_PREFIX", "dns-failover")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected invalid etcd key prefix error")
	}
}

func TestParseListRejectsDuplicates(t *testing.T) {
	t.Parallel()

	_, err := parseList("10.0.0.1:2379,10.0.0.1:2379", "TEST_LIST")
	if err == nil {
		t.Fatal("expected duplicate list value error")
	}
}

func setMinimalEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/healthz")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "example")
}

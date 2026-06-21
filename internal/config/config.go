package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	RegionID       string
	Endpoints      []Endpoint
	DNSTargets     []DNSTarget
	RegionPriority []string
	ServiceRecords []string
	HealthTimeout  time.Duration
	CheckInterval  time.Duration
	Etcd           EtcdConfig
	DNSProvider    DNSProviderConfig
	Notifications  NotificationConfig
}

type Endpoint struct {
	RegionID string
	URL      string
}

type DNSTarget struct {
	RegionID string
	Name     string
}

type DNSProviderConfig struct {
	Provider        string
	APIToken        string
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	ZoneID          string
	RecordID        string
	RecordName      string
	RecordType      string
	TTL             int
}

type EtcdConfig struct {
	Endpoints []string
	KeyPrefix string
}

type NotificationConfig struct {
	SlackWebhookURL string
}

func LoadFromEnv() (Config, error) {
	cfg := Config{
		RegionID:      strings.TrimSpace(os.Getenv("DNS_FAILOVER_REGION_ID")),
		HealthTimeout: 2 * time.Second,
		CheckInterval: 10 * time.Second,
		Etcd: EtcdConfig{
			KeyPrefix: "/dns-failover/",
		},
		DNSProvider: DNSProviderConfig{
			Provider:        strings.TrimSpace(os.Getenv("DNS_FAILOVER_DNS_PROVIDER")),
			APIToken:        os.Getenv("DNS_FAILOVER_DNS_API_TOKEN"),
			AccountID:       strings.TrimSpace(os.Getenv("DNS_FAILOVER_DNS_ACCOUNT_ID")),
			AccessKeyID:     firstNonEmpty(os.Getenv("DNS_FAILOVER_DNS_ACCESS_KEY_ID"), os.Getenv("AWS_ACCESS_KEY_ID")),
			SecretAccessKey: firstNonEmpty(os.Getenv("DNS_FAILOVER_DNS_SECRET_ACCESS_KEY"), os.Getenv("AWS_SECRET_ACCESS_KEY")),
			ZoneID:          os.Getenv("DNS_FAILOVER_DNS_ZONE_ID"),
			RecordID:        os.Getenv("DNS_FAILOVER_DNS_RECORD_ID"),
			RecordName:      os.Getenv("DNS_FAILOVER_DNS_RECORD_NAME"),
			RecordType:      strings.TrimSpace(os.Getenv("DNS_FAILOVER_DNS_RECORD_TYPE")),
			TTL:             60,
		},
		Notifications: NotificationConfig{
			SlackWebhookURL: strings.TrimSpace(os.Getenv("DNS_FAILOVER_SLACK_WEBHOOK_URL")),
		},
	}
	if cfg.DNSProvider.RecordType == "" {
		cfg.DNSProvider.RecordType = "CNAME"
	}

	if ttlText := strings.TrimSpace(os.Getenv("DNS_FAILOVER_DNS_TTL")); ttlText != "" {
		ttl, err := strconv.Atoi(ttlText)
		if err != nil {
			return Config{}, fmt.Errorf("parse DNS_FAILOVER_DNS_TTL: %w", err)
		}
		if ttl <= 0 {
			return Config{}, fmt.Errorf("DNS_FAILOVER_DNS_TTL must be positive")
		}
		cfg.DNSProvider.TTL = ttl
	}

	if cfg.RegionID == "" {
		return Config{}, fmt.Errorf("DNS_FAILOVER_REGION_ID is required")
	}
	if cfg.DNSProvider.Provider == "" {
		return Config{}, fmt.Errorf("DNS_FAILOVER_DNS_PROVIDER is required")
	}

	if timeoutText := strings.TrimSpace(os.Getenv("DNS_FAILOVER_HEALTH_TIMEOUT")); timeoutText != "" {
		timeout, err := time.ParseDuration(timeoutText)
		if err != nil {
			return Config{}, fmt.Errorf("parse DNS_FAILOVER_HEALTH_TIMEOUT: %w", err)
		}
		if timeout <= 0 {
			return Config{}, fmt.Errorf("DNS_FAILOVER_HEALTH_TIMEOUT must be positive")
		}
		cfg.HealthTimeout = timeout
	}

	if intervalText := strings.TrimSpace(os.Getenv("DNS_FAILOVER_CHECK_INTERVAL")); intervalText != "" {
		interval, err := time.ParseDuration(intervalText)
		if err != nil {
			return Config{}, fmt.Errorf("parse DNS_FAILOVER_CHECK_INTERVAL: %w", err)
		}
		if interval <= 0 {
			return Config{}, fmt.Errorf("DNS_FAILOVER_CHECK_INTERVAL must be positive")
		}
		cfg.CheckInterval = interval
	}

	etcdEndpoints, err := parseList(os.Getenv("DNS_FAILOVER_ETCD_ENDPOINTS"), "DNS_FAILOVER_ETCD_ENDPOINTS")
	if err != nil {
		return Config{}, err
	}
	cfg.Etcd.Endpoints = etcdEndpoints
	if keyPrefix := strings.TrimSpace(os.Getenv("DNS_FAILOVER_ETCD_KEY_PREFIX")); keyPrefix != "" {
		if !strings.HasPrefix(keyPrefix, "/") {
			return Config{}, fmt.Errorf("DNS_FAILOVER_ETCD_KEY_PREFIX must start with /")
		}
		if !strings.HasSuffix(keyPrefix, "/") {
			keyPrefix += "/"
		}
		cfg.Etcd.KeyPrefix = keyPrefix
	}

	endpoints, err := parseEndpoints(os.Getenv("DNS_FAILOVER_REGION_ENDPOINTS"))
	if err != nil {
		return Config{}, err
	}
	cfg.Endpoints = endpoints

	dnsTargets, err := parseDNSTargets(os.Getenv("DNS_FAILOVER_REGION_DNS_TARGETS"))
	if err != nil {
		return Config{}, err
	}
	if err := validateRegionSets(endpoints, dnsTargets); err != nil {
		return Config{}, err
	}
	cfg.DNSTargets = dnsTargets

	regionPriority, err := parseRegionPriority(os.Getenv("DNS_FAILOVER_REGION_PRIORITY"))
	if err != nil {
		return Config{}, err
	}
	if err := validateRegionPriority(regionPriority, endpoints); err != nil {
		return Config{}, err
	}
	cfg.RegionPriority = regionPriority

	serviceRecords, err := parseDNSNames(os.Getenv("DNS_FAILOVER_SERVICE_RECORDS"), "DNS_FAILOVER_SERVICE_RECORDS")
	if err != nil {
		return Config{}, err
	}
	cfg.ServiceRecords = serviceRecords

	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func parseEndpoints(raw string) ([]Endpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("DNS_FAILOVER_REGION_ENDPOINTS is required")
	}

	parts := strings.Split(raw, ",")
	endpoints := make([]Endpoint, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("endpoint %q must use region_id=url format", part)
		}

		regionID := strings.TrimSpace(key)
		endpointURL := strings.TrimSpace(value)
		if regionID == "" || endpointURL == "" {
			return nil, fmt.Errorf("endpoint %q has empty region_id or url", part)
		}
		if _, exists := seen[regionID]; exists {
			return nil, fmt.Errorf("duplicate endpoint region_id %q", regionID)
		}

		parsedURL, err := url.ParseRequestURI(endpointURL)
		if err != nil {
			return nil, fmt.Errorf("parse endpoint url for %q: %w", regionID, err)
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			return nil, fmt.Errorf("endpoint %q must use http or https", regionID)
		}
		if parsedURL.Host == "" {
			return nil, fmt.Errorf("endpoint %q must include host", regionID)
		}

		seen[regionID] = struct{}{}
		endpoints = append(endpoints, Endpoint{
			RegionID: regionID,
			URL:      endpointURL,
		})
	}

	return endpoints, nil
}

func parseDNSTargets(raw string) ([]DNSTarget, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("DNS_FAILOVER_REGION_DNS_TARGETS is required")
	}

	parts := strings.Split(raw, ",")
	targets := make([]DNSTarget, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			return nil, fmt.Errorf("dns target %q must use region_id=dns_name format", part)
		}

		regionID := strings.TrimSpace(key)
		name := strings.TrimSuffix(strings.TrimSpace(value), ".")
		if regionID == "" || name == "" {
			return nil, fmt.Errorf("dns target %q has empty region_id or dns_name", part)
		}
		if strings.ContainsAny(name, "/:") {
			return nil, fmt.Errorf("dns target %q must be a DNS name, not a URL", regionID)
		}
		if _, exists := seen[regionID]; exists {
			return nil, fmt.Errorf("duplicate dns target region_id %q", regionID)
		}

		seen[regionID] = struct{}{}
		targets = append(targets, DNSTarget{
			RegionID: regionID,
			Name:     name,
		})
	}

	return targets, nil
}

func validateRegionSets(endpoints []Endpoint, targets []DNSTarget) error {
	endpointRegions := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		endpointRegions[endpoint.RegionID] = struct{}{}
	}

	for _, target := range targets {
		if _, ok := endpointRegions[target.RegionID]; !ok {
			return fmt.Errorf("dns target %q has no matching health endpoint", target.RegionID)
		}
		delete(endpointRegions, target.RegionID)
	}

	for regionID := range endpointRegions {
		return fmt.Errorf("health endpoint %q has no matching dns target", regionID)
	}

	return nil
}

func parseRegionPriority(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("DNS_FAILOVER_REGION_PRIORITY is required")
	}

	parts := strings.Split(raw, ",")
	priority := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		regionID := strings.TrimSpace(part)
		if regionID == "" {
			return nil, fmt.Errorf("DNS_FAILOVER_REGION_PRIORITY contains empty region_id")
		}
		if _, exists := seen[regionID]; exists {
			return nil, fmt.Errorf("duplicate priority region_id %q", regionID)
		}

		seen[regionID] = struct{}{}
		priority = append(priority, regionID)
	}

	return priority, nil
}

func validateRegionPriority(priority []string, endpoints []Endpoint) error {
	endpointRegions := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		endpointRegions[endpoint.RegionID] = struct{}{}
	}

	for _, regionID := range priority {
		if _, ok := endpointRegions[regionID]; !ok {
			return fmt.Errorf("priority region %q has no matching health endpoint", regionID)
		}
		delete(endpointRegions, regionID)
	}

	for regionID := range endpointRegions {
		return fmt.Errorf("health endpoint %q has no matching priority entry", regionID)
	}

	return nil
}

func parseDNSNames(raw string, envName string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		name := strings.TrimSuffix(strings.TrimSpace(part), ".")
		if name == "" {
			return nil, fmt.Errorf("%s contains empty DNS name", envName)
		}
		if strings.ContainsAny(name, "/:") {
			return nil, fmt.Errorf("%s value %q must be a DNS name, not a URL", envName, name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("%s contains duplicate DNS name %q", envName, name)
		}

		seen[name] = struct{}{}
		names = append(names, name)
	}

	return names, nil
}

func parseList(raw string, envName string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			return nil, fmt.Errorf("%s contains empty value", envName)
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("%s contains duplicate value %q", envName, value)
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}

	return values, nil
}

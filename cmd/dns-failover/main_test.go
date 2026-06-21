package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/yangs1202/dns-failover/internal/config"
	"github.com/yangs1202/dns-failover/internal/dnsprovider"
	"github.com/yangs1202/dns-failover/internal/failover"
	"github.com/yangs1202/dns-failover/internal/health"
	"github.com/yangs1202/dns-failover/internal/notify"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type fakeChecker struct {
	results map[string]health.Result
}

func (c fakeChecker) Check(_ context.Context, endpoint config.Endpoint) health.Result {
	if result, ok := c.results[endpoint.RegionID]; ok {
		return result
	}
	return health.Result{RegionID: endpoint.RegionID, Healthy: false, Err: errors.New("missing fake result")}
}

type fakeStore struct {
	putObservations []failover.Observation
	observations    []failover.Observation
	activeDecision  failover.Decision
	activeExists    bool
	leader          bool
	leadershipErr   error
	putErr          error
	putDecision     failover.Decision
	putDecisionOK   bool
	afterLeadership func()
}

func (s *fakeStore) PutObservation(_ context.Context, observation failover.Observation) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.putObservations = append(s.putObservations, observation)
	return nil
}

func (s *fakeStore) Observations(context.Context) ([]failover.Observation, error) {
	return s.observations, nil
}

func (s *fakeStore) ActiveDecision(context.Context) (failover.Decision, bool, error) {
	return s.activeDecision, s.activeExists, nil
}

func (s *fakeStore) PutActiveDecision(_ context.Context, decision failover.Decision) error {
	s.putDecision = decision
	s.putDecisionOK = true
	return nil
}

func (s *fakeStore) WithLeadership(ctx context.Context, _ time.Duration, fn func(context.Context) error) (bool, error) {
	if !s.leader {
		return false, nil
	}
	if s.leadershipErr != nil {
		return true, s.leadershipErr
	}
	err := fn(ctx)
	if s.afterLeadership != nil {
		s.afterLeadership()
	}
	return true, err
}

type fakeProvider struct {
	change dnsprovider.CNAMEChange
	called bool
	err    error
}

func (p *fakeProvider) UpdateCNAME(_ context.Context, change dnsprovider.CNAMEChange) error {
	p.change = change
	p.called = true
	return p.err
}

type fakeNotifier struct {
	events []notify.Event
}

func (n *fakeNotifier) Notify(_ context.Context, event notify.Event) error {
	n.events = append(n.events, event)
	return nil
}

func TestRunFailoverCycleUpdatesDNSWhenLeaderDecisionChanges(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := &fakeStore{
		leader:       true,
		observations: healthyObservations(),
	}
	provider := &fakeProvider{}
	notifier := &fakeNotifier{}

	runFailoverCycle(context.Background(), discardLogger(), testChecker(), store, provider, notifier, cfg)

	if len(store.putObservations) != 2 {
		t.Fatalf("expected 2 stored observations, got %d", len(store.putObservations))
	}
	if !provider.called {
		t.Fatal("expected DNS provider to be called")
	}
	if provider.change.RecordName != "vip.example.invalid" {
		t.Fatalf("expected record name vip.example.invalid, got %q", provider.change.RecordName)
	}
	if provider.change.TargetName != "region-a.example.invalid" {
		t.Fatalf("expected target region-a.example.invalid, got %q", provider.change.TargetName)
	}
	if !store.putDecisionOK || store.putDecision.RegionID != "region-a" {
		t.Fatalf("expected active decision to be stored, got %+v", store.putDecision)
	}
	if len(notifier.events) != 1 || notifier.events[0].Title != "dns-failover target updated" {
		t.Fatalf("expected update notification, got %+v", notifier.events)
	}
}

func TestRunFailoverCycleSkipsDNSWhenActiveDecisionMatches(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := &fakeStore{
		leader:         true,
		observations:   healthyObservations(),
		activeExists:   true,
		activeDecision: failover.Decision{RegionID: "region-a", TargetName: "region-a.example.invalid"},
	}
	provider := &fakeProvider{}
	notifier := &fakeNotifier{}

	runFailoverCycle(context.Background(), discardLogger(), testChecker(), store, provider, notifier, cfg)

	if provider.called {
		t.Fatal("expected DNS provider not to be called")
	}
	if store.putDecisionOK {
		t.Fatal("expected active decision not to be rewritten")
	}
	if len(notifier.events) != 0 {
		t.Fatalf("expected no notifications, got %+v", notifier.events)
	}
}

func TestRunFailoverCycleNotifiesLeaderDecisionFailure(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := &fakeStore{
		leader:       true,
		observations: nil,
	}
	notifier := &fakeNotifier{}

	runFailoverCycle(context.Background(), discardLogger(), testChecker(), store, &fakeProvider{}, notifier, cfg)

	if len(notifier.events) != 1 {
		t.Fatalf("expected failure notification, got %d", len(notifier.events))
	}
	if notifier.events[0].Title != "dns-failover decision failed" {
		t.Fatalf("expected failure title, got %q", notifier.events[0].Title)
	}
	if notifier.events[0].Fields["error"] == "" {
		t.Fatalf("expected failure error field, got %+v", notifier.events[0].Fields)
	}
}

func TestRunFailoverCycleSkipsWhenNotLeader(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := &fakeStore{leader: false}
	provider := &fakeProvider{}
	notifier := &fakeNotifier{}

	runFailoverCycle(context.Background(), discardLogger(), testChecker(), store, provider, notifier, cfg)

	if provider.called {
		t.Fatal("expected provider not to be called")
	}
	if len(notifier.events) != 0 {
		t.Fatalf("expected no notifications, got %+v", notifier.events)
	}
}

func TestRunFailoverCycleStopsOnObservationWriteError(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := &fakeStore{leader: true, putErr: errors.New("put failed")}
	provider := &fakeProvider{}

	runFailoverCycle(context.Background(), discardLogger(), testChecker(), store, provider, &fakeNotifier{}, cfg)

	if provider.called {
		t.Fatal("expected provider not to be called")
	}
}

func TestRunFailoverCycleNotifiesProviderError(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	store := &fakeStore{
		leader:       true,
		observations: healthyObservations(),
	}
	provider := &fakeProvider{err: errors.New("provider failed")}
	notifier := &fakeNotifier{}

	runFailoverCycle(context.Background(), discardLogger(), testChecker(), store, provider, notifier, cfg)

	if !provider.called {
		t.Fatal("expected provider to be called")
	}
	if len(notifier.events) != 1 {
		t.Fatalf("expected failure notification, got %+v", notifier.events)
	}
	if notifier.events[0].Title != "dns-failover decision failed" {
		t.Fatalf("expected failure notification, got %+v", notifier.events[0])
	}
}

func TestObservationTTLUsesMinimums(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.CheckInterval = time.Second
	cfg.HealthTimeout = 2 * time.Second
	cfg.Endpoints = append(cfg.Endpoints, config.Endpoint{RegionID: "region-c", URL: "https://c.example.invalid/healthz"})

	if got := observationTTL(cfg); got != 30*time.Second {
		t.Fatalf("expected minimum ttl 30s, got %s", got)
	}

	cfg.CheckInterval = 20 * time.Second
	if got := observationTTL(cfg); got != time.Minute {
		t.Fatalf("expected ttl 1m, got %s", got)
	}
}

func TestErrorString(t *testing.T) {
	t.Parallel()

	if got := errorString(nil); got != "" {
		t.Fatalf("expected empty nil error string, got %q", got)
	}
	if got := errorString(errors.New("boom")); got != "boom" {
		t.Fatalf("expected error string, got %q", got)
	}
}

func TestNewNotifier(t *testing.T) {
	t.Parallel()

	noWebhook, err := newNotifier(config.Config{})
	if err != nil {
		t.Fatalf("newNotifier returned error: %v", err)
	}
	if _, ok := noWebhook.(notify.NoopNotifier); !ok {
		t.Fatalf("expected NoopNotifier, got %T", noWebhook)
	}

	withWebhook, err := newNotifier(config.Config{
		Notifications: config.NotificationConfig{SlackWebhookURL: "https://example.invalid/webhook"},
	})
	if err != nil {
		t.Fatalf("newNotifier returned error: %v", err)
	}
	if _, ok := withWebhook.(notify.SlackNotifier); !ok {
		t.Fatalf("expected SlackNotifier, got %T", withWebhook)
	}
}

func TestNewDNSProviderCreatesCloudflareProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      config.DNSProviderConfig
		wantType any
	}{
		{
			name: "cloudflare",
			cfg: config.DNSProviderConfig{
				Provider:   "cloudflare",
				APIToken:   "token",
				ZoneID:     "zone-1",
				RecordID:   "record-1",
				RecordName: "vip.example.invalid",
				RecordType: "CNAME",
				TTL:        60,
			},
			wantType: dnsprovider.CloudflareProvider{},
		},
		{
			name: "digitalocean",
			cfg: config.DNSProviderConfig{
				Provider:   "digitalocean",
				APIToken:   "token",
				ZoneID:     "example.invalid",
				RecordID:   "record-1",
				RecordName: "vip.example.invalid",
				RecordType: "CNAME",
				TTL:        60,
			},
			wantType: dnsprovider.DigitalOceanProvider{},
		},
		{
			name: "dnsimple",
			cfg: config.DNSProviderConfig{
				Provider:   "dnsimple",
				APIToken:   "token",
				AccountID:  "123",
				ZoneID:     "example.invalid",
				RecordID:   "record-1",
				RecordName: "vip.example.invalid",
				RecordType: "CNAME",
				TTL:        60,
			},
			wantType: dnsprovider.DNSimpleProvider{},
		},
		{
			name: "route53",
			cfg: config.DNSProviderConfig{
				Provider:        "route53",
				ZoneID:          "Z123",
				RecordName:      "vip.example.invalid",
				RecordType:      "CNAME",
				AccessKeyID:     "access",
				SecretAccessKey: "secret",
				TTL:             60,
			},
			wantType: dnsprovider.Route53Provider{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := newDNSProvider(config.Config{DNSProvider: tt.cfg})
			if err != nil {
				t.Fatalf("newDNSProvider returned error: %v", err)
			}
			if got, want := typeName(provider), typeName(tt.wantType); got != want {
				t.Fatalf("expected %s, got %s", want, got)
			}
		})
	}
}

func TestNewDNSProviderRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := newDNSProvider(config.Config{
		DNSProvider: config.DNSProviderConfig{Provider: "missing"},
	})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestRunRejectsMissingConfig(t *testing.T) {
	t.Setenv("DNS_FAILOVER_REGION_ID", "")

	err := run(context.Background(), discardLogger())
	if err == nil {
		t.Fatal("expected config error")
	}
}

func TestRunRejectsMissingEtcdEndpoints(t *testing.T) {
	setMinimalRunEnv(t)

	err := run(context.Background(), discardLogger())
	if err == nil {
		t.Fatal("expected missing etcd endpoints error")
	}
	if err.Error() != "etcd endpoints are required for automatic failover" {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestRunRejectsProviderConfigBeforeConnectingEtcd(t *testing.T) {
	setMinimalRunEnv(t)
	t.Setenv("DNS_FAILOVER_ETCD_ENDPOINTS", "127.0.0.1:2379")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "cloudflare")

	err := run(context.Background(), discardLogger())
	if err == nil {
		t.Fatal("expected provider config error")
	}
}

func TestRunWithDependenciesRunsCycleAndStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	store := &fakeStore{
		leader:          true,
		observations:    healthyObservations(),
		afterLeadership: cancel,
	}
	provider := &fakeProvider{}
	notifier := &fakeNotifier{}

	err := runWithDependencies(ctx, discardLogger(), agentDependencies{
		loadConfig: func() (config.Config, error) {
			cfg := testConfig()
			cfg.Etcd.Endpoints = []string{"127.0.0.1:2379"}
			return cfg, nil
		},
		newProvider: func(config.Config) (dnsprovider.Provider, error) {
			return provider, nil
		},
		newNotifier: func(config.Config) (notify.Notifier, error) {
			return notifier, nil
		},
		newEtcdClient: func(config.Config) (*clientv3.Client, error) {
			return nil, nil
		},
		newStore: func(*clientv3.Client, config.Config) failoverStore {
			return store
		},
		newChecker: func(config.Config) healthChecker {
			return testChecker()
		},
	})
	if err != nil {
		t.Fatalf("runWithDependencies returned error: %v", err)
	}
	if !provider.called {
		t.Fatal("expected provider to be called")
	}
	if len(notifier.events) != 1 {
		t.Fatalf("expected update notification, got %+v", notifier.events)
	}
}

func TestRunWithDependenciesReturnsDependencyErrors(t *testing.T) {
	t.Parallel()

	baseDeps := func() agentDependencies {
		return agentDependencies{
			loadConfig: func() (config.Config, error) {
				cfg := testConfig()
				cfg.Etcd.Endpoints = []string{"127.0.0.1:2379"}
				return cfg, nil
			},
			newProvider: func(config.Config) (dnsprovider.Provider, error) {
				return &fakeProvider{}, nil
			},
			newNotifier: func(config.Config) (notify.Notifier, error) {
				return &fakeNotifier{}, nil
			},
			newEtcdClient: func(config.Config) (*clientv3.Client, error) {
				return nil, nil
			},
			newStore: func(*clientv3.Client, config.Config) failoverStore {
				return &fakeStore{leader: false}
			},
			newChecker: func(config.Config) healthChecker {
				return testChecker()
			},
		}
	}

	tests := []struct {
		name   string
		mutate func(*agentDependencies)
	}{
		{
			name: "load config",
			mutate: func(deps *agentDependencies) {
				deps.loadConfig = func() (config.Config, error) {
					return config.Config{}, errors.New("load")
				}
			},
		},
		{
			name: "provider",
			mutate: func(deps *agentDependencies) {
				deps.newProvider = func(config.Config) (dnsprovider.Provider, error) {
					return nil, errors.New("provider")
				}
			},
		},
		{
			name: "notifier",
			mutate: func(deps *agentDependencies) {
				deps.newNotifier = func(config.Config) (notify.Notifier, error) {
					return nil, errors.New("notifier")
				}
			},
		},
		{
			name: "etcd client",
			mutate: func(deps *agentDependencies) {
				deps.newEtcdClient = func(config.Config) (*clientv3.Client, error) {
					return nil, errors.New("etcd")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := baseDeps()
			tt.mutate(&deps)

			err := runWithDependencies(context.Background(), discardLogger(), deps)
			if err == nil {
				t.Fatal("expected dependency error")
			}
		})
	}
}

func testConfig() config.Config {
	return config.Config{
		RegionID: "region-a",
		Endpoints: []config.Endpoint{
			{RegionID: "region-a", URL: "https://a.example.invalid/healthz"},
			{RegionID: "region-b", URL: "https://b.example.invalid/healthz"},
		},
		DNSTargets: []config.DNSTarget{
			{RegionID: "region-a", Name: "region-a.example.invalid"},
			{RegionID: "region-b", Name: "region-b.example.invalid"},
		},
		RegionPriority: []string{"region-a", "region-b"},
		HealthTimeout:  time.Second,
		CheckInterval:  10 * time.Second,
		DNSProvider: config.DNSProviderConfig{
			ZoneID:     "zone-1",
			RecordID:   "record-1",
			RecordName: "vip.example.invalid",
		},
	}
}

func setMinimalRunEnv(t *testing.T) {
	t.Helper()

	t.Setenv("DNS_FAILOVER_REGION_ID", "region-a")
	t.Setenv("DNS_FAILOVER_REGION_ENDPOINTS", "region-a=https://region-a.example.invalid/healthz")
	t.Setenv("DNS_FAILOVER_REGION_DNS_TARGETS", "region-a=region-a.example.invalid")
	t.Setenv("DNS_FAILOVER_REGION_PRIORITY", "region-a")
	t.Setenv("DNS_FAILOVER_DNS_PROVIDER", "unsupported")
}

func testChecker() fakeChecker {
	return fakeChecker{
		results: map[string]health.Result{
			"region-a": {RegionID: "region-a", Healthy: true, StatusCode: 200},
			"region-b": {RegionID: "region-b", Healthy: false, StatusCode: 503, Err: errors.New("unhealthy")},
		},
	}
}

func healthyObservations() []failover.Observation {
	return []failover.Observation{
		{ObserverRegion: "region-a", TargetRegion: "region-a", Healthy: true},
		{ObserverRegion: "region-b", TargetRegion: "region-a", Healthy: true},
		{ObserverRegion: "region-a", TargetRegion: "region-b", Healthy: false},
		{ObserverRegion: "region-b", TargetRegion: "region-b", Healthy: false},
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func typeName(value any) string {
	return fmt.Sprintf("%T", value)
}

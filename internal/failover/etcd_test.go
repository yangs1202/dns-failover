package failover

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"go.etcd.io/etcd/server/v3/embed"
)

func TestEtcdStorePersistsObservationsAndActiveDecision(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)

	observation := Observation{
		ObserverRegion: "region-a",
		TargetRegion:   "region-b",
		Healthy:        true,
		StatusCode:     200,
		ObservedAt:     time.Now().UTC(),
	}
	if err := store.PutObservation(context.Background(), observation); err != nil {
		t.Fatalf("PutObservation returned error: %v", err)
	}

	observations, err := store.Observations(context.Background())
	if err != nil {
		t.Fatalf("Observations returned error: %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(observations))
	}
	if observations[0].ObserverRegion != observation.ObserverRegion || observations[0].TargetRegion != observation.TargetRegion {
		t.Fatalf("unexpected observation %+v", observations[0])
	}

	decision := Decision{RegionID: "region-b", TargetName: "region-b.example.invalid"}
	if err := store.PutActiveDecision(context.Background(), decision); err != nil {
		t.Fatalf("PutActiveDecision returned error: %v", err)
	}

	got, exists, err := store.ActiveDecision(context.Background())
	if err != nil {
		t.Fatalf("ActiveDecision returned error: %v", err)
	}
	if !exists {
		t.Fatal("expected active decision to exist")
	}
	if got != decision {
		t.Fatalf("expected decision %+v, got %+v", decision, got)
	}
}

func TestEtcdStoreActiveDecisionMissing(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)

	_, exists, err := store.ActiveDecision(context.Background())
	if err != nil {
		t.Fatalf("ActiveDecision returned error: %v", err)
	}
	if exists {
		t.Fatal("expected no active decision")
	}
}

func TestEtcdStoreWithLeadership(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)
	called := false

	leader, err := store.WithLeadership(context.Background(), time.Second, func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithLeadership returned error: %v", err)
	}
	if !leader {
		t.Fatal("expected leadership lock to be acquired")
	}
	if !called {
		t.Fatal("expected leadership callback to be called")
	}
}

func TestEtcdStoreWithLeadershipReturnsCallbackError(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)
	wantErr := errors.New("callback failed")

	leader, err := store.WithLeadership(context.Background(), time.Second, func(context.Context) error {
		return wantErr
	})
	if !leader {
		t.Fatal("expected leadership lock to be acquired")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected callback error, got %v", err)
	}
}

func TestEtcdStoreWithLeadershipSkipsWhenLocked(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)

	session, err := concurrency.NewSession(client, concurrency.WithTTL(1))
	if err != nil {
		t.Fatalf("create lock session: %v", err)
	}
	defer session.Close()
	mutex := concurrency.NewMutex(session, store.key("leader"))
	if err := mutex.Lock(context.Background()); err != nil {
		t.Fatalf("lock leader mutex: %v", err)
	}
	defer func() {
		_ = mutex.Unlock(context.Background())
	}()

	called := false
	leader, err := store.WithLeadership(context.Background(), time.Second, func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithLeadership returned error: %v", err)
	}
	if leader {
		t.Fatal("expected leadership to be skipped")
	}
	if called {
		t.Fatal("expected callback not to be called")
	}
}

func TestEtcdStoreObservationsRejectInvalidJSON(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)

	if _, err := client.Put(context.Background(), store.key("observations", "bad"), "{"); err != nil {
		t.Fatalf("seed invalid observation: %v", err)
	}

	_, err := store.Observations(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestEtcdStoreActiveDecisionRejectInvalidJSON(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)

	if _, err := client.Put(context.Background(), store.key("active"), "{"); err != nil {
		t.Fatalf("seed invalid active decision: %v", err)
	}

	_, _, err := store.ActiveDecision(context.Background())
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestEtcdStoreReturnsClientErrors(t *testing.T) {
	t.Parallel()

	client := startEmbeddedEtcd(t)
	if err := client.Close(); err != nil {
		t.Fatalf("close etcd client: %v", err)
	}
	store := NewEtcdStore(client, "/dns-failover-test/", time.Second)

	if err := store.PutObservation(context.Background(), Observation{ObserverRegion: "a", TargetRegion: "b"}); err == nil {
		t.Fatal("expected PutObservation client error")
	}
	if _, err := store.Observations(context.Background()); err == nil {
		t.Fatal("expected Observations client error")
	}
	if err := store.PutActiveDecision(context.Background(), Decision{RegionID: "a", TargetName: "a.example.invalid"}); err == nil {
		t.Fatal("expected PutActiveDecision client error")
	}
	if _, _, err := store.ActiveDecision(context.Background()); err == nil {
		t.Fatal("expected ActiveDecision client error")
	}
	if _, err := store.WithLeadership(context.Background(), time.Second, func(context.Context) error {
		return nil
	}); err == nil {
		t.Fatal("expected WithLeadership client error")
	}
}

func TestEtcdStoreKeyKeepsLeadingSlash(t *testing.T) {
	t.Parallel()

	store := NewEtcdStore(nil, "/dns-failover-test/", time.Second)
	if got := store.key("observations", "region-a", "region-b"); got != "/dns-failover-test/observations/region-a/region-b" {
		t.Fatalf("unexpected key %q", got)
	}

	noLeadingSlash := NewEtcdStore(nil, "dns-failover-test/", 100*time.Millisecond)
	if noLeadingSlash.leaseTTL != 1 {
		t.Fatalf("expected minimum lease ttl 1, got %d", noLeadingSlash.leaseTTL)
	}
	if got := noLeadingSlash.key("active"); got != "dns-failover-test/active" {
		t.Fatalf("unexpected key %q", got)
	}
}

func startEmbeddedEtcd(t *testing.T) *clientv3.Client {
	t.Helper()

	clientURL := freeLocalURL(t)
	peerURL := freeLocalURL(t)

	cfg := embed.NewConfig()
	cfg.Name = "default"
	cfg.Dir = t.TempDir()
	cfg.LogLevel = "error"
	cfg.LogOutputs = []string{"stderr"}
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.InitialCluster = "default=" + peerURL.String()

	server, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start embedded etcd: %v", err)
	}
	t.Cleanup(server.Close)

	select {
	case <-server.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		server.Server.Stop()
		t.Fatal("embedded etcd did not become ready")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{clientURL.String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("create etcd client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	return client
}

func freeLocalURL(t *testing.T) url.URL {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate local port: %v", err)
	}
	defer listener.Close()

	raw := "http://" + listener.Addr().String()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse local url %q: %v", raw, err)
	}
	return *parsed
}

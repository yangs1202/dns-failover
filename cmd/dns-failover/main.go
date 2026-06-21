package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/yangs1202/dns-failover/internal/config"
	"github.com/yangs1202/dns-failover/internal/dnsprovider"
	"github.com/yangs1202/dns-failover/internal/failover"
	"github.com/yangs1202/dns-failover/internal/health"
	"github.com/yangs1202/dns-failover/internal/notify"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}
	if len(cfg.Etcd.Endpoints) == 0 {
		logger.Error("etcd endpoints are required for automatic failover")
		os.Exit(1)
	}

	provider, err := newDNSProvider(cfg)
	if err != nil {
		logger.Error("create dns provider failed", "error", err)
		os.Exit(1)
	}
	notifier, err := newNotifier(cfg)
	if err != nil {
		logger.Error("create notifier failed", "error", err)
		os.Exit(1)
	}

	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Etcd.Endpoints,
		DialTimeout: cfg.HealthTimeout,
	})
	if err != nil {
		logger.Error("create etcd client failed", "error", err)
		os.Exit(1)
	}
	defer etcdClient.Close()

	checker := health.NewHTTPChecker(cfg.HealthTimeout)
	store := failover.NewEtcdStore(etcdClient, cfg.Etcd.KeyPrefix, observationTTL(cfg))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info(
		"agent started",
		"region", cfg.RegionID,
		"check_interval", cfg.CheckInterval.String(),
		"health_timeout", cfg.HealthTimeout.String(),
		"etcd_endpoints", len(cfg.Etcd.Endpoints),
		"etcd_key_prefix", cfg.Etcd.KeyPrefix,
		"dns_provider", cfg.DNSProvider.Provider,
		"slack_notifications", cfg.Notifications.SlackWebhookURL != "",
	)

	runFailoverCycle(ctx, logger, checker, store, provider, notifier, cfg)

	ticker := time.NewTicker(cfg.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("agent stopped", "region", cfg.RegionID)
			return
		case <-ticker.C:
			runFailoverCycle(ctx, logger, checker, store, provider, notifier, cfg)
		}
	}
}

func runFailoverCycle(ctx context.Context, logger *slog.Logger, checker health.HTTPChecker, store failover.EtcdStore, provider dnsprovider.Provider, notifier notify.Notifier, cfg config.Config) {
	observations := runHealthCheckCycle(ctx, logger, checker, cfg)
	for _, observation := range observations {
		if err := store.PutObservation(ctx, observation); err != nil {
			logger.Error(
				"write health observation failed",
				"observer_region", observation.ObserverRegion,
				"target_region", observation.TargetRegion,
				"error", err,
			)
			return
		}
	}

	leader, err := store.WithLeadership(ctx, cfg.CheckInterval, func(leaderCtx context.Context) error {
		observations, err := store.Observations(leaderCtx)
		if err != nil {
			return err
		}

		decision, err := failover.Decide(observations, cfg.RegionPriority, cfg.DNSTargets)
		if err != nil {
			return err
		}

		activeDecision, exists, err := store.ActiveDecision(leaderCtx)
		if err != nil {
			return err
		}
		if exists && activeDecision.RegionID == decision.RegionID && activeDecision.TargetName == decision.TargetName {
			logger.Info(
				"active dns target already selected",
				"leader_region", cfg.RegionID,
				"active_region", decision.RegionID,
				"target_name", decision.TargetName,
			)
			return nil
		}

		if err := provider.UpdateCNAME(leaderCtx, dnsprovider.CNAMEChange{
			ZoneID:     cfg.DNSProvider.ZoneID,
			RecordID:   cfg.DNSProvider.RecordID,
			RecordName: cfg.DNSProvider.RecordName,
			TargetName: decision.TargetName,
		}); err != nil {
			return err
		}
		if err := store.PutActiveDecision(leaderCtx, decision); err != nil {
			return err
		}

		logger.Info(
			"dns failover target updated",
			"leader_region", cfg.RegionID,
			"active_region", decision.RegionID,
			"target_name", decision.TargetName,
		)
		if err := notifier.Notify(leaderCtx, notify.Event{
			Title: "dns-failover target updated",
			Fields: map[string]string{
				"leader_region": cfg.RegionID,
				"active_region": decision.RegionID,
				"target_name":   decision.TargetName,
				"record_name":   cfg.DNSProvider.RecordName,
			},
		}); err != nil {
			logger.Error("send failover notification failed", "error", err)
		}
		return nil
	})
	if err != nil {
		logger.Error("failover decision failed", "leader", leader, "error", err)
		if leader {
			if notifyErr := notifier.Notify(ctx, notify.Event{
				Title: "dns-failover decision failed",
				Fields: map[string]string{
					"leader_region": cfg.RegionID,
					"record_name":   cfg.DNSProvider.RecordName,
					"error":         err.Error(),
				},
			}); notifyErr != nil {
				logger.Error("send failover failure notification failed", "error", notifyErr)
			}
		}
		return
	}
	if !leader {
		logger.Info("failover decision skipped by non-leader", "observer_region", cfg.RegionID)
	}
}

func runHealthCheckCycle(ctx context.Context, logger *slog.Logger, checker health.HTTPChecker, cfg config.Config) []failover.Observation {
	cycleCtx, cancel := context.WithTimeout(ctx, cfg.HealthTimeout*time.Duration(len(cfg.Endpoints)))
	defer cancel()

	observations := make([]failover.Observation, 0, len(cfg.Endpoints))
	for _, endpoint := range cfg.Endpoints {
		result := checker.Check(cycleCtx, endpoint)
		observation := failover.ObservationFromResult(cfg.RegionID, result, time.Now())
		observations = append(observations, observation)
		logger.Info(
			"health observation",
			"observer_region", cfg.RegionID,
			"target_region", endpoint.RegionID,
			"healthy", result.Healthy,
			"status_code", result.StatusCode,
			"latency_ms", result.Latency.Milliseconds(),
			"error", errorString(result.Err),
		)
	}

	logger.Info("health check cycle completed", "observer_region", cfg.RegionID)
	return observations
}

func newNotifier(cfg config.Config) (notify.Notifier, error) {
	if cfg.Notifications.SlackWebhookURL == "" {
		return notify.NoopNotifier{}, nil
	}

	return notify.NewSlackNotifier(cfg.Notifications.SlackWebhookURL)
}

func newDNSProvider(cfg config.Config) (dnsprovider.Provider, error) {
	registry := dnsprovider.NewRegistry()
	if err := registry.Register("cloudflare", dnsprovider.NewCloudflareProvider); err != nil {
		return nil, err
	}

	return registry.NewProvider(dnsprovider.Config{
		Name:       cfg.DNSProvider.Provider,
		APIToken:   cfg.DNSProvider.APIToken,
		ZoneID:     cfg.DNSProvider.ZoneID,
		RecordID:   cfg.DNSProvider.RecordID,
		RecordName: cfg.DNSProvider.RecordName,
		RecordType: cfg.DNSProvider.RecordType,
	})
}

func observationTTL(cfg config.Config) time.Duration {
	ttl := cfg.CheckInterval * 3
	minimumTTL := cfg.HealthTimeout*time.Duration(len(cfg.Endpoints)) + cfg.CheckInterval
	if ttl < minimumTTL {
		ttl = minimumTTL
	}
	if ttl < 30*time.Second {
		ttl = 30 * time.Second
	}
	return ttl
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

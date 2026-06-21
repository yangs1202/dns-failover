package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/yangs1202/dns-failover/internal/config"
	"github.com/yangs1202/dns-failover/internal/health"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("load config failed", "error", err)
		os.Exit(1)
	}

	checker := health.NewHTTPChecker(cfg.HealthTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.HealthTimeout*time.Duration(len(cfg.Endpoints)))
	defer cancel()

	for _, endpoint := range cfg.Endpoints {
		result := checker.Check(ctx, endpoint)
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

	fmt.Println("health check cycle completed")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

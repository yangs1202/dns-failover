package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/yangs1202/dns-failover/internal/config"
)

type Result struct {
	RegionID   string
	Healthy    bool
	StatusCode int
	Latency    time.Duration
	Err        error
}

type HTTPChecker struct {
	client *http.Client
}

func NewHTTPChecker(timeout time.Duration) HTTPChecker {
	return HTTPChecker{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c HTTPChecker) Check(ctx context.Context, endpoint config.Endpoint) Result {
	startedAt := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URL, nil)
	if err != nil {
		return Result{
			RegionID: endpoint.RegionID,
			Latency:  time.Since(startedAt),
			Err:      fmt.Errorf("create request: %w", err),
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return Result{
			RegionID: endpoint.RegionID,
			Latency:  time.Since(startedAt),
			Err:      fmt.Errorf("perform request: %w", err),
		}
	}
	defer resp.Body.Close()

	return Result{
		RegionID:   endpoint.RegionID,
		Healthy:    resp.StatusCode == http.StatusOK,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(startedAt),
	}
}

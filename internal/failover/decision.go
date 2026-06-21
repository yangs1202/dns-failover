package failover

import (
	"fmt"
	"time"

	"github.com/yangs1202/dns-failover/internal/config"
	"github.com/yangs1202/dns-failover/internal/health"
)

type Observation struct {
	ObserverRegion string    `json:"observer_region"`
	TargetRegion   string    `json:"target_region"`
	Healthy        bool      `json:"healthy"`
	StatusCode     int       `json:"status_code"`
	Error          string    `json:"error,omitempty"`
	ObservedAt     time.Time `json:"observed_at"`
}

type Decision struct {
	RegionID   string
	TargetName string
}

func ObservationFromResult(observerRegion string, result health.Result, observedAt time.Time) Observation {
	return Observation{
		ObserverRegion: observerRegion,
		TargetRegion:   result.RegionID,
		Healthy:        result.Healthy,
		StatusCode:     result.StatusCode,
		Error:          errorString(result.Err),
		ObservedAt:     observedAt.UTC(),
	}
}

func Decide(observations []Observation, priority []string, targets []config.DNSTarget) (Decision, error) {
	targetByRegion := make(map[string]string, len(targets))
	for _, target := range targets {
		targetByRegion[target.RegionID] = target.Name
	}

	activeObservers := make(map[string]struct{})
	healthyVotes := make(map[string]int)
	for _, observation := range observations {
		activeObservers[observation.ObserverRegion] = struct{}{}
		if observation.Healthy {
			healthyVotes[observation.TargetRegion]++
		}
	}
	if len(activeObservers) == 0 {
		return Decision{}, fmt.Errorf("no active observations")
	}

	requiredVotes := len(activeObservers)/2 + 1
	for _, regionID := range priority {
		targetName, ok := targetByRegion[regionID]
		if !ok {
			return Decision{}, fmt.Errorf("priority region %q has no DNS target", regionID)
		}
		if healthyVotes[regionID] >= requiredVotes {
			return Decision{
				RegionID:   regionID,
				TargetName: targetName,
			}, nil
		}
	}

	return Decision{}, fmt.Errorf("no healthy failover target: active_observers=%d required_votes=%d", len(activeObservers), requiredVotes)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

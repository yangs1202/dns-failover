package failover

import (
	"errors"
	"testing"
	"time"

	"github.com/yangs1202/dns-failover/internal/config"
	"github.com/yangs1202/dns-failover/internal/health"
)

func TestDecideSelectsFirstHealthyPriorityRegion(t *testing.T) {
	t.Parallel()

	observedAt := time.Now()
	decision, err := Decide([]Observation{
		{ObserverRegion: "gs", TargetRegion: "gs", Healthy: true, ObservedAt: observedAt},
		{ObserverRegion: "sg", TargetRegion: "gs", Healthy: true, ObservedAt: observedAt},
		{ObserverRegion: "gs", TargetRegion: "sg", Healthy: true, ObservedAt: observedAt},
		{ObserverRegion: "sg", TargetRegion: "sg", Healthy: true, ObservedAt: observedAt},
	}, []string{"gs", "sg"}, []config.DNSTarget{
		{RegionID: "gs", Name: "gs.example.invalid"},
		{RegionID: "sg", Name: "sg.example.invalid"},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.RegionID != "gs" {
		t.Fatalf("expected gs, got %q", decision.RegionID)
	}
}

func TestDecideFailsOverToNextHealthyRegion(t *testing.T) {
	t.Parallel()

	observedAt := time.Now()
	decision, err := Decide([]Observation{
		{ObserverRegion: "gs", TargetRegion: "gs", Healthy: false, ObservedAt: observedAt},
		{ObserverRegion: "sg", TargetRegion: "gs", Healthy: false, ObservedAt: observedAt},
		{ObserverRegion: "gs", TargetRegion: "sg", Healthy: true, ObservedAt: observedAt},
		{ObserverRegion: "sg", TargetRegion: "sg", Healthy: true, ObservedAt: observedAt},
	}, []string{"gs", "sg"}, []config.DNSTarget{
		{RegionID: "gs", Name: "gs.example.invalid"},
		{RegionID: "sg", Name: "sg.example.invalid"},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.RegionID != "sg" {
		t.Fatalf("expected sg, got %q", decision.RegionID)
	}
}

func TestDecideUsesCurrentObserverMajority(t *testing.T) {
	t.Parallel()

	decision, err := Decide([]Observation{
		{ObserverRegion: "sg", TargetRegion: "gs", Healthy: false, ObservedAt: time.Now()},
		{ObserverRegion: "sg", TargetRegion: "sg", Healthy: true, ObservedAt: time.Now()},
	}, []string{"gs", "sg"}, []config.DNSTarget{
		{RegionID: "gs", Name: "gs.example.invalid"},
		{RegionID: "sg", Name: "sg.example.invalid"},
	})
	if err != nil {
		t.Fatalf("Decide returned error: %v", err)
	}
	if decision.RegionID != "sg" {
		t.Fatalf("expected sg, got %q", decision.RegionID)
	}
}

func TestDecideRejectsNoHealthyTarget(t *testing.T) {
	t.Parallel()

	_, err := Decide([]Observation{
		{ObserverRegion: "gs", TargetRegion: "gs", Healthy: false, ObservedAt: time.Now()},
	}, []string{"gs"}, []config.DNSTarget{
		{RegionID: "gs", Name: "gs.example.invalid"},
	})
	if err == nil {
		t.Fatal("expected no healthy target error")
	}
}

func TestDecideRejectsNoObservations(t *testing.T) {
	t.Parallel()

	_, err := Decide(nil, []string{"gs"}, []config.DNSTarget{{RegionID: "gs", Name: "gs.example.invalid"}})
	if err == nil {
		t.Fatal("expected no active observations error")
	}
}

func TestDecideRejectsPriorityWithoutTarget(t *testing.T) {
	t.Parallel()

	_, err := Decide([]Observation{
		{ObserverRegion: "gs", TargetRegion: "gs", Healthy: true, ObservedAt: time.Now()},
	}, []string{"gs"}, nil)
	if err == nil {
		t.Fatal("expected missing DNS target error")
	}
}

func TestObservationFromResult(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 6, 21, 12, 0, 0, 0, time.FixedZone("KST", 9*60*60))
	observation := ObservationFromResult("observer-a", health.Result{
		RegionID:   "target-a",
		Healthy:    false,
		StatusCode: 503,
		Err:        errors.New("boom"),
	}, observedAt)

	if observation.ObserverRegion != "observer-a" {
		t.Fatalf("expected observer-a, got %q", observation.ObserverRegion)
	}
	if observation.TargetRegion != "target-a" {
		t.Fatalf("expected target-a, got %q", observation.TargetRegion)
	}
	if observation.Error != "boom" {
		t.Fatalf("expected error boom, got %q", observation.Error)
	}
	if observation.ObservedAt.Location() != time.UTC {
		t.Fatalf("expected UTC observed time, got %s", observation.ObservedAt.Location())
	}

	healthy := ObservationFromResult("observer-a", health.Result{RegionID: "target-a", Healthy: true}, observedAt)
	if healthy.Error != "" {
		t.Fatalf("expected empty error, got %q", healthy.Error)
	}
}

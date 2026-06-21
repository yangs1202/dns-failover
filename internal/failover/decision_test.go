package failover

import (
	"testing"
	"time"

	"github.com/yangs1202/dns-failover/internal/config"
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

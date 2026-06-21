package dnsprovider

import (
	"context"
	"testing"
)

type fakeProvider struct{}

func (fakeProvider) UpdateCNAME(context.Context, CNAMEChange) error {
	return nil
}

func TestRegistryCreatesRegisteredProvider(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	if err := registry.Register("fake", func(Config) (Provider, error) {
		return fakeProvider{}, nil
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	provider, err := registry.NewProvider(Config{Name: "fake"})
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider")
	}
}

func TestRegistryRejectsUnsupportedProvider(t *testing.T) {
	t.Parallel()

	_, err := NewRegistry().NewProvider(Config{Name: "missing"})
	if err == nil {
		t.Fatal("expected unsupported provider error")
	}
}

func TestRegistryRejectsDuplicateAndEmptyProvider(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	if err := registry.Register("", func(Config) (Provider, error) {
		return fakeProvider{}, nil
	}); err == nil {
		t.Fatal("expected empty provider name error")
	}
	if err := registry.Register("fake", func(Config) (Provider, error) {
		return fakeProvider{}, nil
	}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	if err := registry.Register("fake", func(Config) (Provider, error) {
		return fakeProvider{}, nil
	}); err == nil {
		t.Fatal("expected duplicate provider error")
	}
}

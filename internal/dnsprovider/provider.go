package dnsprovider

import (
	"context"
	"fmt"
)

type Provider interface {
	UpdateCNAME(ctx context.Context, change CNAMEChange) error
}

type CNAMEChange struct {
	ZoneID     string
	RecordID   string
	RecordName string
	TargetName string
}

type Config struct {
	Name            string
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

type Factory func(Config) (Provider, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() Registry {
	return Registry{
		factories: make(map[string]Factory),
	}
}

func (r Registry) Register(name string, factory Factory) error {
	if name == "" {
		return fmt.Errorf("dns provider name is required")
	}
	if factory == nil {
		return fmt.Errorf("dns provider factory for %q is required", name)
	}
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("dns provider %q is already registered", name)
	}

	r.factories[name] = factory
	return nil
}

func (r Registry) NewProvider(cfg Config) (Provider, error) {
	factory, ok := r.factories[cfg.Name]
	if !ok {
		return nil, fmt.Errorf("unsupported dns provider %q", cfg.Name)
	}

	return factory(cfg)
}

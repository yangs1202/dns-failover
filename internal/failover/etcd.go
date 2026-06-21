package failover

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

type EtcdStore struct {
	client    *clientv3.Client
	keyPrefix string
	leaseTTL  int64
}

func NewEtcdStore(client *clientv3.Client, keyPrefix string, leaseTTL time.Duration) EtcdStore {
	if leaseTTL < time.Second {
		leaseTTL = time.Second
	}
	return EtcdStore{
		client:    client,
		keyPrefix: keyPrefix,
		leaseTTL:  int64(leaseTTL.Seconds()),
	}
}

func (s EtcdStore) PutObservation(ctx context.Context, observation Observation) error {
	body, err := json.Marshal(observation)
	if err != nil {
		return fmt.Errorf("marshal observation: %w", err)
	}

	lease, err := s.client.Grant(ctx, s.leaseTTL)
	if err != nil {
		return fmt.Errorf("grant observation lease: %w", err)
	}

	key := s.key("observations", observation.ObserverRegion, observation.TargetRegion)
	if _, err := s.client.Put(ctx, key, string(body), clientv3.WithLease(lease.ID)); err != nil {
		return fmt.Errorf("put observation: %w", err)
	}

	return nil
}

func (s EtcdStore) Observations(ctx context.Context) ([]Observation, error) {
	resp, err := s.client.Get(ctx, s.key("observations")+"/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("get observations: %w", err)
	}

	observations := make([]Observation, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var observation Observation
		if err := json.Unmarshal(kv.Value, &observation); err != nil {
			return nil, fmt.Errorf("decode observation %q: %w", string(kv.Key), err)
		}
		observations = append(observations, observation)
	}

	return observations, nil
}

func (s EtcdStore) PutActiveDecision(ctx context.Context, decision Decision) error {
	body, err := json.Marshal(struct {
		RegionID   string    `json:"region_id"`
		TargetName string    `json:"target_name"`
		UpdatedAt  time.Time `json:"updated_at"`
	}{
		RegionID:   decision.RegionID,
		TargetName: decision.TargetName,
		UpdatedAt:  time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal active decision: %w", err)
	}

	if _, err := s.client.Put(ctx, s.key("active"), string(body)); err != nil {
		return fmt.Errorf("put active decision: %w", err)
	}

	return nil
}

func (s EtcdStore) ActiveDecision(ctx context.Context) (Decision, bool, error) {
	resp, err := s.client.Get(ctx, s.key("active"))
	if err != nil {
		return Decision{}, false, fmt.Errorf("get active decision: %w", err)
	}
	if len(resp.Kvs) == 0 {
		return Decision{}, false, nil
	}

	var stored struct {
		RegionID   string `json:"region_id"`
		TargetName string `json:"target_name"`
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, &stored); err != nil {
		return Decision{}, false, fmt.Errorf("decode active decision: %w", err)
	}

	return Decision{
		RegionID:   stored.RegionID,
		TargetName: stored.TargetName,
	}, true, nil
}

func (s EtcdStore) WithLeadership(ctx context.Context, ttl time.Duration, fn func(context.Context) error) (bool, error) {
	if ttl < time.Second {
		ttl = time.Second
	}

	session, err := concurrency.NewSession(s.client, concurrency.WithTTL(int(ttl.Seconds())), concurrency.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("create etcd leadership session: %w", err)
	}
	defer session.Close()

	mutex := concurrency.NewMutex(session, s.key("leader"))
	lockCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if err := mutex.TryLock(lockCtx); err != nil {
		if errors.Is(err, concurrency.ErrLocked) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return false, nil
		}
		return false, fmt.Errorf("acquire etcd leadership lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mutex.Unlock(unlockCtx)
	}()

	if err := fn(ctx); err != nil {
		return true, err
	}

	return true, nil
}

func (s EtcdStore) key(parts ...string) string {
	joined := path.Join(append([]string{s.keyPrefix}, parts...)...)
	if s.keyPrefix[0] == '/' && joined[0] != '/' {
		joined = "/" + joined
	}
	return joined
}

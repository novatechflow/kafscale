package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdStoreConfig defines how we connect to etcd for metadata/offsets.
type EtcdStoreConfig struct {
	Endpoints   []string
	Username    string
	Password    string
	DialTimeout time.Duration
}

// EtcdStore uses etcd for offset persistence while delegating metadata to an in-memory snapshot.
type EtcdStore struct {
	client   *clientv3.Client
	metadata *InMemoryStore
	cancel   context.CancelFunc
}

// NewEtcdStore initializes a store backed by etcd.
func NewEtcdStore(ctx context.Context, snapshot ClusterMetadata, cfg EtcdStoreConfig) (*EtcdStore, error) {
	if len(cfg.Endpoints) == 0 {
		return nil, errors.New("etcd endpoints required")
	}
	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = 5 * time.Second
	}
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		Username:    cfg.Username,
		Password:    cfg.Password,
		DialTimeout: cfg.DialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("connect etcd: %w", err)
	}
store := &EtcdStore{
	client:   cli,
	metadata: NewInMemoryStore(snapshot),
}
if err := store.refreshSnapshot(ctx); err != nil {
	// ignore if snapshot missing; operator will populate later
}
store.startWatchers()
return store, nil
}

// Metadata delegates to the snapshot captured at startup (operator keeps it fresh).
func (s *EtcdStore) Metadata(ctx context.Context, topics []string) (*ClusterMetadata, error) {
	return s.metadata.Metadata(ctx, topics)
}

// NextOffset reads the last committed offset from etcd and returns the next offset to assign.
func (s *EtcdStore) NextOffset(ctx context.Context, topic string, partition int32) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	key := offsetKey(topic, partition)
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return 0, err
	}
	if len(resp.Kvs) == 0 {
		return 0, nil
	}
	val := strings.TrimSpace(string(resp.Kvs[0].Value))
	if val == "" {
		return 0, nil
	}
	offset, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse offset for %s: %w", key, err)
	}
	return offset, nil
}

// UpdateOffsets stores the next offset (last + 1) so future producers pick up from there.
func (s *EtcdStore) UpdateOffsets(ctx context.Context, topic string, partition int32, lastOffset int64) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	next := lastOffset + 1
	_, err := s.client.Put(ctx, offsetKey(topic, partition), strconv.FormatInt(next, 10))
	return err
}

func offsetKey(topic string, partition int32) string {
	return fmt.Sprintf("/kafscale/topics/%s/partitions/%d/next_offset", topic, partition)
}

func (s *EtcdStore) startWatchers() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go s.watchSnapshot(ctx)
}

func (s *EtcdStore) watchSnapshot(ctx context.Context) {
	watchChan := s.client.Watch(ctx, snapshotKey(), clientv3.WithPrefix())
	for resp := range watchChan {
		if resp.Err() != nil {
			continue
		}
		if err := s.refreshSnapshot(ctx); err != nil {
			continue
		}
	}
}

func (s *EtcdStore) refreshSnapshot(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := s.client.Get(ctx, snapshotKey())
	if err != nil {
		return err
	}
	if len(resp.Kvs) == 0 {
		return nil
	}
	var snapshot ClusterMetadata
	if err := json.Unmarshal(resp.Kvs[0].Value, &snapshot); err != nil {
		return err
	}
	s.metadata.Update(snapshot)
	return nil
}

func snapshotKey() string {
	return "/kafscale/metadata/snapshot"
}

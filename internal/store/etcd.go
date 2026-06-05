package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdStore wraps an etcd client connection.
type EtcdStore struct {
	Client *clientv3.Client
}

// NewEtcdStore creates and returns a new EtcdStore connected to the given endpoints.
func NewEtcdStore(ctx context.Context, endpoints []string, dialTimeout time.Duration) (*EtcdStore, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   endpoints,
		DialTimeout: dialTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("connect etcd: %w", err)
	}

	// Verify connection is actually working
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	defer pingCancel()
	_, err = cli.Get(pingCtx, "/", clientv3.WithCountOnly())
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("ping etcd: %w", err)
	}

	slog.Info("已连接到 etcd", "endpoints", endpoints)
	return &EtcdStore{Client: cli}, nil
}

// Close closes the underlying etcd client connection.
func (e *EtcdStore) Close() error {
	return e.Client.Close()
}

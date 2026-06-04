package election

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
)

// LeaderElection manages leader election using etcd concurrency primitives.
type LeaderElection struct {
	client   *clientv3.Client
	prefix   string
	session  *concurrency.Session
	election *concurrency.Election
	isLeader atomic.Bool
}

// NewLeaderElection creates a new LeaderElection instance with an etcd session.
func NewLeaderElection(client *clientv3.Client, prefix string, ttl int) (*LeaderElection, error) {
	session, err := concurrency.NewSession(client, concurrency.WithTTL(ttl))
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &LeaderElection{
		client:   client,
		prefix:   prefix,
		session:  session,
		election: concurrency.NewElection(session, prefix),
	}, nil
}

// Campaign blocks until this instance becomes the leader or ctx is cancelled.
func (le *LeaderElection) Campaign(ctx context.Context) error {
	slog.Info("开始竞选 Leader", "prefix", le.prefix)
	if err := le.election.Campaign(ctx, ""); err != nil {
		le.isLeader.Store(false)
		return fmt.Errorf("campaign: %w", err)
	}
	le.isLeader.Store(true)
	slog.Info("已成为 Leader")
	// Block until session expires or ctx cancelled
	<-le.session.Done()
	le.isLeader.Store(false)
	slog.Warn("Session 过期，失去 Leader")
	return fmt.Errorf("session expired")
}

// IsLeader returns true if this instance is currently the leader.
func (le *LeaderElection) IsLeader() bool {
	return le.isLeader.Load()
}

// Resign voluntarily steps down as leader.
func (le *LeaderElection) Resign(ctx context.Context) error {
	if err := le.election.Resign(ctx); err != nil {
		return fmt.Errorf("resign: %w", err)
	}
	le.isLeader.Store(false)
	return nil
}

// Close closes the underlying etcd session.
func (le *LeaderElection) Close() error {
	return le.session.Close()
}

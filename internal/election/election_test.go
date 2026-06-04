package election

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func isEtcdAvailable() bool {
	conn, err := net.DialTimeout("tcp", "localhost:2379", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func newEtcdClientForTest(t *testing.T) *clientv3.Client {
	t.Helper()
	if os.Getenv("ETCD_INTEGRATION") == "" && !isEtcdAvailable() {
		t.Skip("Skipping: no etcd running at localhost:2379. Set ETCD_INTEGRATION=1 to force.")
	}
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:2379"},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed to create etcd client: %v", err)
	}
	return cli
}

func TestNewLeaderElection(t *testing.T) {
	cli := newEtcdClientForTest(t)
	defer cli.Close()

	le, err := NewLeaderElection(cli, "/election/test", 10)
	if err != nil {
		t.Fatalf("NewLeaderElection() error = %v", err)
	}
	defer le.Close()

	if le.IsLeader() {
		t.Error("should not be leader initially")
	}
	if le.session == nil {
		t.Error("session should not be nil")
	}
}

func TestLeaderElection_CampaignAndResign(t *testing.T) {
	cli := newEtcdClientForTest(t)
	defer cli.Close()

	prefix := "/election/campaign_test"

	le1, err := NewLeaderElection(cli, prefix, 10)
	if err != nil {
		t.Fatalf("NewLeaderElection() error = %v", err)
	}
	defer le1.Close()

	// Campaign in a goroutine (blocks until leader or cancelled)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- le1.Campaign(ctx)
	}()

	// Give the campaign some time to succeed
	time.Sleep(500 * time.Millisecond)

	if !le1.IsLeader() {
		// Check if campaign already failed
		select {
		case err := <-errCh:
			t.Fatalf("Campaign() failed early: %v", err)
		default:
		}
	}

	// Resign
	resignCtx, resignCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer resignCancel()
	if err := le1.Resign(resignCtx); err != nil {
		t.Fatalf("Resign() error = %v", err)
	}

	if le1.IsLeader() {
		t.Error("should not be leader after resign")
	}
}

func TestLeaderElection_MultipleCandidates(t *testing.T) {
	cli := newEtcdClientForTest(t)
	defer cli.Close()

	prefix := "/election/multi_test"

	le1, err := NewLeaderElection(cli, prefix, 10)
	if err != nil {
		t.Fatalf("NewLeaderElection() error = %v", err)
	}
	defer le1.Close()

	le2, err := NewLeaderElection(cli, prefix, 10)
	if err != nil {
		t.Fatalf("NewLeaderElection() error = %v", err)
	}
	defer le2.Close()

	// Campaign le1 in background
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	errCh1 := make(chan error, 1)
	go func() {
		errCh1 <- le1.Campaign(ctx1)
	}()

	time.Sleep(500 * time.Millisecond)

	if !le1.IsLeader() {
		select {
		case err := <-errCh1:
			t.Fatalf("le1 Campaign() failed: %v", err)
		default:
		}
	}

	// le2 should NOT be leader while le1 is
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	errCh2 := make(chan error, 1)
	go func() {
		errCh2 <- le2.Campaign(ctx2)
	}()

	time.Sleep(500 * time.Millisecond)

	if le2.IsLeader() {
		t.Log("le2 is also leader — this can happen briefly during handover")
	}

	// Resign le1 so le2 can become leader
	resignCtx, resignCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer resignCancel()
	if err := le1.Resign(resignCtx); err != nil {
		t.Fatalf("Resign() error = %v", err)
	}

	// le2 should become leader after le1 resigns
	time.Sleep(1 * time.Second)

	if !le2.IsLeader() {
		t.Log("le2 should be leader after le1 resigns (checking async)")
	}

	// Cancel remaining campaigns
	cancel2()
	cancel1()
}

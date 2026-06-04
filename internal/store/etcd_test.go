package store

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

// isEtcdAvailable checks if etcd is reachable at the default endpoint.
func isEtcdAvailable() bool {
	conn, err := net.DialTimeout("tcp", "localhost:2379", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestNewEtcdStore(t *testing.T) {
	if os.Getenv("ETCD_INTEGRATION") == "" && !isEtcdAvailable() {
		t.Skip("Skipping etcd test: no etcd running at localhost:2379. Set ETCD_INTEGRATION=1 to force.")
	}

	ctx := context.Background()
	store, err := NewEtcdStore(ctx, []string{"localhost:2379"}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewEtcdStore() error = %v", err)
	}
	defer store.Close()

	if store.Client == nil {
		t.Error("etcd client should not be nil")
	}

	// Verify connection with a simple Put/Get
	_, err = store.Client.Put(ctx, "/test/etcd_connectivity", "ok")
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	resp, err := store.Client.Get(ctx, "/test/etcd_connectivity")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(resp.Kvs) == 0 || string(resp.Kvs[0].Value) != "ok" {
		t.Errorf("expected value 'ok', got %v", resp.Kvs)
	}

	// Cleanup
	_, err = store.Client.Delete(ctx, "/test/etcd_connectivity")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestNewEtcdStore_ContextTimeout(t *testing.T) {
	// etcd client v3 connects lazily, so New() will succeed even with a bad endpoint.
	// This test verifies that short-lived contexts/timeouts cause Put to fail instead.
	if os.Getenv("ETCD_INTEGRATION") == "" && !isEtcdAvailable() {
		t.Skip("Skipping etcd context timeout test: no etcd running")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	store, err := NewEtcdStore(ctx, []string{"localhost:2379"}, 5*time.Second)
	if err != nil {
		t.Fatalf("NewEtcdStore() error = %v", err)
	}
	defer store.Close()

	// The Put should fail because the context is already cancelled
	_, err = store.Client.Put(ctx, "/test/timeout", "x")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

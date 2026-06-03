package grpcclient

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/shangyizhou/mini-bk/pkg/proto"
)

// startTestServer starts a minimal gRPC server for testing pool connections
func startTestServer(t *testing.T) (addr string, stop func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	proto.RegisterAgentServiceServer(srv, &testAgentServer{})
	go srv.Serve(lis)

	return lis.Addr().String(), func() {
		srv.GracefulStop()
	}
}

// testAgentServer implements a minimal AgentServiceServer for testing
type testAgentServer struct {
	proto.UnimplementedAgentServiceServer
}

func TestPool_GetOrCreate(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	pool := NewPool()
	ctx := context.Background()

	// First call should create a new connection
	client, err := pool.GetOrCreate(ctx, "test-node-1", addr)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if client == nil {
		t.Fatal("GetOrCreate() returned nil client")
	}

	if pool.Len() != 1 {
		t.Errorf("pool.Len() = %d, want 1", pool.Len())
	}

	// Second call with same nodeID should return existing client
	client2, err := pool.GetOrCreate(ctx, "test-node-1", addr)
	if err != nil {
		t.Fatalf("GetOrCreate() second call error = %v", err)
	}
	// Should be the same client
	if client != client2 {
		t.Error("GetOrCreate() returned different client for same nodeID")
	}

	if pool.Len() != 1 {
		t.Errorf("pool.Len() = %d, want 1 (no new connections)", pool.Len())
	}
}

func TestPool_GetOrCreate_MultipleNodes(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	pool := NewPool()
	ctx := context.Background()

	client1, err := pool.GetOrCreate(ctx, "node-1", addr)
	if err != nil {
		t.Fatalf("GetOrCreate node-1 error = %v", err)
	}

	client2, err := pool.GetOrCreate(ctx, "node-2", addr)
	if err != nil {
		t.Fatalf("GetOrCreate node-2 error = %v", err)
	}

	if client1 == client2 {
		t.Error("different nodeIDs should return different clients")
	}

	if pool.Len() != 2 {
		t.Errorf("pool.Len() = %d, want 2", pool.Len())
	}
}

func TestPool_Remove(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	pool := NewPool()
	ctx := context.Background()

	_, err := pool.GetOrCreate(ctx, "test-node", addr)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}

	if pool.Len() != 1 {
		t.Fatalf("pool.Len() = %d, want 1 before remove", pool.Len())
	}

	pool.Remove("test-node")
	if pool.Len() != 0 {
		t.Errorf("pool.Len() = %d, want 0 after remove", pool.Len())
	}

	// Re-add should work
	_, err = pool.GetOrCreate(ctx, "test-node", addr)
	if err != nil {
		t.Fatalf("GetOrCreate() after remove error = %v", err)
	}
	if pool.Len() != 1 {
		t.Errorf("pool.Len() = %d, want 1 after re-add", pool.Len())
	}
}

func TestPool_Close(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	pool := NewPool()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		nodeID := fmt.Sprintf("node-%d", i)
		_, err := pool.GetOrCreate(ctx, nodeID, addr)
		if err != nil {
			t.Fatalf("GetOrCreate %s error = %v", nodeID, err)
		}
	}

	if pool.Len() != 3 {
		t.Fatalf("pool.Len() = %d, want 3 before close", pool.Len())
	}

	pool.Close()
	if pool.Len() != 0 {
		t.Errorf("pool.Len() = %d, want 0 after close", pool.Len())
	}
}

func TestPool_ConcurrentAccess(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	pool := NewPool()
	ctx := context.Background()

	// Concurrently create connections
	done := make(chan struct{})
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 3; j++ {
				nodeID := fmt.Sprintf("node-%d", j)
				_, err := pool.GetOrCreate(ctx, nodeID, addr)
				if err != nil {
					t.Errorf("concurrent GetOrCreate error = %v", err)
				}
			}
			done <- struct{}{}
		}()
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	if pool.Len() != 3 {
		t.Errorf("pool.Len() = %d, want 3 after concurrent access", pool.Len())
	}

	pool.Close()
}

func TestPool_DialTimeout(t *testing.T) {
	pool := NewPool()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Try connecting to a non-routable address to test timeout
	_, err := pool.GetOrCreate(ctx, "unreachable-node", "10.255.255.1:1")
	if err == nil {
		t.Skip("dial to unreachable address succeeded unexpectedly (network may be proxied)")
	}
}

func TestPool_GetOrCreate_ServerNotStarted(t *testing.T) {
	// Test with a port that's likely not in use
	pool := NewPool()
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := pool.GetOrCreate(ctx, "no-server", "127.0.0.1:15999")
	if err == nil {
		t.Error("expected error when connecting to non-existent server")
	}
}

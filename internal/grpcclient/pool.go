package grpcclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/shangyizhou/mini-bk/pkg/proto"
)

// Pool manages gRPC connections to agents
type Pool struct {
	mu      sync.RWMutex
	clients map[string]proto.AgentServiceClient // nodeID -> client
	conns   map[string]*grpc.ClientConn         // nodeID -> connection
	opts    []grpc.DialOption
}

// NewPool creates a new gRPC connection pool
func NewPool() *Pool {
	return &Pool{
		clients: make(map[string]proto.AgentServiceClient),
		conns:   make(map[string]*grpc.ClientConn),
		opts: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithTimeout(5 * time.Second),
		},
	}
}

// GetOrCreate returns an existing client for the node or creates a new connection
func (p *Pool) GetOrCreate(ctx context.Context, nodeID, addr string) (proto.AgentServiceClient, error) {
	p.mu.RLock()
	if c, ok := p.clients[nodeID]; ok {
		p.mu.RUnlock()
		return c, nil
	}
	p.mu.RUnlock()

	conn, err := grpc.DialContext(ctx, addr, p.opts...)
	if err != nil {
		return nil, fmt.Errorf("dial agent %s at %s: %w", nodeID, addr, err)
	}

	client := proto.NewAgentServiceClient(conn)

	p.mu.Lock()
	// Double-check after acquiring write lock
	if existing, ok := p.clients[nodeID]; ok {
		p.mu.Unlock()
		conn.Close() // Close the redundant connection
		return existing, nil
	}
	p.clients[nodeID] = client
	p.conns[nodeID] = conn
	p.mu.Unlock()

	return client, nil
}

// Remove closes and removes the connection for a node
func (p *Pool) Remove(nodeID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if conn, ok := p.conns[nodeID]; ok {
		conn.Close()
		delete(p.conns, nodeID)
	}
	delete(p.clients, nodeID)
}

// Close closes all connections in the pool
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, conn := range p.conns {
		conn.Close()
	}
	p.conns = make(map[string]*grpc.ClientConn)
	p.clients = make(map[string]proto.AgentServiceClient)
}

// Len returns the number of active connections
func (p *Pool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.clients)
}

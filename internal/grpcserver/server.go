package grpcserver

import (
	"context"

	"github.com/shangyizhou/mini-bk/internal/nodemanager"
	"github.com/shangyizhou/mini-bk/pkg/proto"
	"google.golang.org/grpc"
)

// AgentServer implements the proto.AgentServiceServer interface
type AgentServer struct {
	proto.UnimplementedAgentServiceServer
	nodeMgr   *nodemanager.NodeManager
}

// NewAgentServer creates a new AgentServer
func NewAgentServer(nodeMgr *nodemanager.NodeManager) *AgentServer {
	return &AgentServer{nodeMgr: nodeMgr}
}

// Register handles agent registration
func (s *AgentServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	return s.nodeMgr.Register(ctx, req)
}

// Heartbeat handles agent heartbeat
func (s *AgentServer) Heartbeat(ctx context.Context, req *proto.HeartbeatRequest) (*proto.HeartbeatResponse, error) {
	err := s.nodeMgr.Heartbeat(ctx, req)
	return &proto.HeartbeatResponse{Ok: err == nil}, err
}

// SubmitTask accepts a task from an agent (server-side, this is called by agents
// to submit tasks — or in the reverse direction, the server pushes tasks to agents)
func (s *AgentServer) SubmitTask(ctx context.Context, req *proto.TaskRequest) (*proto.TaskResponse, error) {
	// Full implementation in Task 5 with scheduler integration
	return &proto.TaskResponse{Accepted: true}, nil
}

// CancelTask cancels a task
func (s *AgentServer) CancelTask(ctx context.Context, req *proto.CancelRequest) (*proto.CancelResponse, error) {
	// Publish cancel signal — full implementation in Task 5
	return &proto.CancelResponse{Ok: true}, nil
}

// StreamLog accepts log chunks from agent
func (s *AgentServer) StreamLog(stream proto.AgentService_StreamLogServer) error {
	// Accept log chunks from agent, full implementation in later phase
	return nil
}

// ReportResult receives task execution result from agent
func (s *AgentServer) ReportResult(ctx context.Context, req *proto.ResultRequest) (*proto.ResultResponse, error) {
	// Update task result — full implementation in Task 5
	return &proto.ResultResponse{Ok: true}, nil
}

// RegisterWithGRPC registers the AgentService with a gRPC server
func RegisterWithGRPC(grpcServer *grpc.Server, srv *AgentServer) {
	proto.RegisterAgentServiceServer(grpcServer, srv)
}

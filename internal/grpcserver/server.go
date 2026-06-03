package grpcserver

import (
	"context"
	"log/slog"

	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/internal/nodemanager"
	"github.com/shangyizhou/mini-bk/pkg/proto"
	"google.golang.org/grpc"
)

// taskDispatcher defines the task dispatch interface needed by the gRPC server.
type taskDispatcher interface {
	GetNextTaskForNode(nodeID string) *model.Task
	HandleRemoteResult(ctx context.Context, taskUID string, result *executor.TaskResult)
}

// AgentServer implements the proto.AgentServiceServer interface
type AgentServer struct {
	proto.UnimplementedAgentServiceServer
	nodeMgr   *nodemanager.NodeManager
	scheduler taskDispatcher
}

// NewAgentServer creates a new AgentServer
func NewAgentServer(nodeMgr *nodemanager.NodeManager, scheduler taskDispatcher) *AgentServer {
	return &AgentServer{nodeMgr: nodeMgr, scheduler: scheduler}
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
	if s.scheduler == nil {
		slog.Warn("grpcserver: no scheduler configured, discarding result", "task_uid", req.TaskUid)
		return &proto.ResultResponse{Ok: false}, nil
	}

	result := &executor.TaskResult{
		ExitCode: int(req.ExitCode),
		Stdout:   req.Stdout,
		Stderr:   req.Stderr,
		TimedOut: req.TimedOut,
	}
	if req.ErrorMessage != "" {
		result.Error = &executor.RemoteError{Message: req.ErrorMessage}
	}

	s.scheduler.HandleRemoteResult(ctx, req.TaskUid, result)
	return &proto.ResultResponse{Ok: true}, nil
}

// PullTask returns a pending task for the agent, if any.
func (s *AgentServer) PullTask(ctx context.Context, req *proto.PullTaskRequest) (*proto.PullTaskResponse, error) {
	if s.scheduler == nil {
		return &proto.PullTaskResponse{}, nil
	}

	task := s.scheduler.GetNextTaskForNode(req.NodeId)
	if task == nil {
		return &proto.PullTaskResponse{}, nil
	}

	return &proto.PullTaskResponse{
		TaskUid:   task.TaskUID,
		Name:      task.Name,
		Command:   task.Command,
		Workdir:   task.Workdir,
		Env:       task.Env,
		TimeoutSec: int32(task.TimeoutSec),
	}, nil
}

// RegisterWithGRPC registers the AgentService with a gRPC server
func RegisterWithGRPC(grpcServer *grpc.Server, srv *AgentServer) {
	proto.RegisterAgentServiceServer(grpcServer, srv)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"math"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/model"
	"github.com/shangyizhou/mini-bk/pkg/proto"
)

var (
	serverAddr = flag.String("server-addr", "localhost:50051", "Server gRPC address")
	labels     = flag.String("labels", "", "Comma-separated node labels")
	version    = "0.3.0"
)

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	hostname, err := os.Hostname()
	if err != nil {
		slog.Error("获取主机名失败", "error", err)
		hostname = "unknown"
	}
	ip := getOutboundIP()

	labelList := []string{}
	if *labels != "" {
		for _, l := range strings.Split(*labels, ",") {
			trimmed := strings.TrimSpace(l)
			if trimmed != "" {
				labelList = append(labelList, trimmed)
			}
		}
	}

	slog.Info("Agent 启动中", "hostname", hostname, "ip", ip, "server", *serverAddr)

	// Connect to Server gRPC
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, *serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		slog.Error("连接 Server 失败", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := proto.NewAgentServiceClient(conn)

	// Detect local resources
	totalCPU := detectCPU()
	totalMemMB := detectMemory()
	totalDiskMB := detectDisk("/")

	slog.Info("本地资源", "cpu", totalCPU, "memory_mb", totalMemMB, "disk_mb", totalDiskMB)

	// Register with Server
	regCtx, regCancel := context.WithTimeout(context.Background(), 10*time.Second)
	resp, err := client.Register(regCtx, &proto.RegisterRequest{
		Hostname:      hostname,
		Ip:            ip,
		Version:       version,
		TotalCpu:      int64(totalCPU),
		TotalMemoryMb: int64(totalMemMB),
		TotalDiskMb:   int64(totalDiskMB),
		Labels:        labelList,
	})
	regCancel()
	if err != nil {
		slog.Error("注册失败", "error", err)
		os.Exit(1)
	}
	if !resp.Accepted {
		slog.Error("注册被拒绝", "message", resp.GetMessage())
		os.Exit(1)
	}
	nodeID := resp.NodeId
	slog.Info("注册成功", "node_id", nodeID)

	// Start heartbeat goroutine
	go heartbeatLoop(client, nodeID, version)

	// Start task polling loop
	go taskPollLoop(client, nodeID)

	// Wait for signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("Agent 正在退出", "signal", sig)
}

func heartbeatLoop(client proto.AgentServiceClient, nodeID, version string) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resources := collectResources()
		_, err := client.Heartbeat(ctx, &proto.HeartbeatRequest{
			NodeId:    nodeID,
			Version:   version,
			Resources: resources,
		})
		cancel()
		if err != nil {
			slog.Warn("心跳发送失败", "error", err)
		}
	}
}

// taskPollLoop periodically polls the server for tasks assigned to this node.
func taskPollLoop(client proto.AgentServiceClient, nodeID string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		resp, err := client.PullTask(ctx, &proto.PullTaskRequest{NodeId: nodeID})
		cancel()
		if err != nil {
			slog.Debug("拉取任务失败", "error", err)
			continue
		}
		if resp.TaskUid == "" {
			continue
		}

		slog.Info("收到远程任务", "task_uid", resp.TaskUid, "name", resp.Name)
		go executeTask(client, nodeID, resp)
	}
}

// executeTask runs a task received via PullTask and reports the result.
func executeTask(client proto.AgentServiceClient, nodeID string, req *proto.PullTaskResponse) {
	task := model.NewTask(req.Name, req.Command)
	task.TaskUID = req.TaskUid
	task.Workdir = req.Workdir
	task.Env = req.Env
	task.TimeoutSec = int(req.TimeoutSec)

	exec := executor.NewExecutor(10, nil)
	ctx := context.Background()
	result := exec.Run(ctx, task)

	slog.Info("远程任务执行完毕",
		"task_uid", req.TaskUid,
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
	)

	reportCtx, reportCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer reportCancel()

	errMsg := ""
	if result.Error != nil {
		errMsg = result.Error.Error()
	}

	_, err := client.ReportResult(reportCtx, &proto.ResultRequest{
		TaskUid:      req.TaskUid,
		ExitCode:     int32(result.ExitCode),
		Stdout:       result.Stdout,
		Stderr:       result.Stderr,
		ErrorMessage: errMsg,
		TimedOut:     result.TimedOut,
	})
	if err != nil {
		slog.Error("报告任务结果失败", "task_uid", req.TaskUid, "error", err)
	}
}

// collectResources gathers current resource usage for heartbeat.
func collectResources() *proto.NodeResource {
	return &proto.NodeResource{
		CpuUsagePercent: getCPUUsage(),
		MemoryUsedMb:    int64(getMemoryUsed()),
		MemoryTotalMb:   int64(detectMemory()),
		DiskUsedMb:      int64(getDiskUsed("/")),
		DiskTotalMb:     int64(detectDisk("/")),
		LoadAvg_1M:      getLoadAvg(),
		RunningTasks:    int32(countRunningTasks()),
	}
}

// detectCPU returns the number of logical CPU cores.
func detectCPU() int {
	return runtime.NumCPU()
}

// getCPUUsage returns a simple CPU usage estimate.
func getCPUUsage() float64 {
	// Simple implementation: measure idle time difference
	// For a minimal agent, return 0 (placeholder)
	return 0.0
}

// detectMemory returns total physical memory in MB.
func detectMemory() int {
	// Try different methods based on platform
	memMB := detectMemorySysctl()
	if memMB > 0 {
		return memMB
	}
	memMB = detectMemoryProc()
	if memMB > 0 {
		return memMB
	}
	// Default fallback
	return 8192
}

// detectMemorySysctl uses sysctl on macOS/BSD to get total memory.
func detectMemorySysctl() int {
	// Darwin: sysctl hw.memsize
	raw, err := syscall.Sysctl("hw.memsize")
	if err != nil {
		return 0
	}
	var memBytes uint64
	if _, err := fmt.Sscanf(raw, "%d", &memBytes); err != nil {
		return 0
	}
	return int(memBytes / 1024 / 1024)
}

// detectMemoryProc reads /proc/meminfo on Linux.
func detectMemoryProc() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb int
			if _, err := fmt.Sscanf(line, "MemTotal: %d kB", &kb); err == nil {
				return kb / 1024
			}
		}
	}
	return 0
}

// getMemoryUsed returns approximate used memory in MB.
func getMemoryUsed() int {
	// Read /proc/meminfo on Linux for a more accurate value
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	var memTotal, memAvailable, memFree int
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
		}
		if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &memAvailable)
		}
		if strings.HasPrefix(line, "MemFree:") && memAvailable == 0 {
			fmt.Sscanf(line, "MemFree: %d kB", &memFree)
		}
	}
	if memAvailable > 0 {
		return (memTotal - memAvailable) / 1024
	}
	if memFree > 0 {
		return (memTotal - memFree) / 1024
	}
	return 0
}

// detectDisk returns total disk space in MB for the given path.
func detectDisk(path string) int {
	fs := syscall.Statfs_t{}
	if err := syscall.Statfs(path, &fs); err != nil {
		return 102400 // default 100GB
	}
	totalBytes := fs.Blocks * uint64(fs.Bsize)
	return int(totalBytes / 1024 / 1024)
}

// getDiskUsed returns used disk space in MB for the given path.
func getDiskUsed(path string) int {
	fs := syscall.Statfs_t{}
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0
	}
	totalBytes := fs.Blocks * uint64(fs.Bsize)
	freeBytes := fs.Bfree * uint64(fs.Bsize)
	return int((totalBytes - freeBytes) / 1024 / 1024)
}

// getLoadAvg returns the 1-minute load average.
func getLoadAvg() float64 {
	raw, err := syscall.Sysctl("vm.loadavg")
	if err != nil {
		// Try /proc/loadavg on Linux
		data, err := os.ReadFile("/proc/loadavg")
		if err != nil {
			return 0.0
		}
		var load1, load5, load15 float64
		if _, err := fmt.Sscanf(string(data), "%f %f %f", &load1, &load5, &load15); err != nil {
			return 0.0
		}
		return math.Round(load1*100) / 100
	}
	// On Darwin, vm.loadavg returns a struct loadavg with 3 doubles
	// We parse it as space-separated values
	var load1 float64
	if _, err := fmt.Sscanf(raw, "{ %f", &load1); err != nil {
		return 0.0
	}
	return math.Round(load1*100) / 100
}

// countRunningTasks returns the count of running processes for this agent.
func countRunningTasks() int {
	// Placeholder: agent-level tracking would maintain this
	return 0
}

// getOutboundIP obtains the preferred outbound IP address of this machine.
func getOutboundIP() string {
	// Dial a UDP connection to determine the preferred outbound IP
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 3*time.Second)
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

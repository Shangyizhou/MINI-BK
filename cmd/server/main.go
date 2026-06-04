package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"

	"github.com/shangyizhou/mini-bk/internal/api"
	"github.com/shangyizhou/mini-bk/internal/config"
	"github.com/shangyizhou/mini-bk/internal/executor"
	"github.com/shangyizhou/mini-bk/internal/grpcserver"
	"github.com/shangyizhou/mini-bk/internal/logstream"
	"github.com/shangyizhou/mini-bk/internal/middleware"
	"github.com/shangyizhou/mini-bk/internal/nodemanager"
	"github.com/shangyizhou/mini-bk/internal/queue"
	"github.com/shangyizhou/mini-bk/internal/scheduler"
	"github.com/shangyizhou/mini-bk/internal/service"
	"github.com/shangyizhou/mini-bk/internal/store"
)

func main() {
	// 初始化结构化日志
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// 加载配置
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "configs/config.yaml"
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("加载配置失败", "error", err)
		os.Exit(1)
	}

	// 连接 PostgreSQL
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pg, err := store.NewPostgres(ctx, cfg.Database.DSN())
	if err != nil {
		slog.Error("连接 PostgreSQL 失败", "error", err)
		os.Exit(1)
	}
	defer pg.Close()

	// 连接 Redis（可选用于日志流、队列等）
	var logStream *logstream.LogStream
	var rdb *redis.Client
	if cfg.Redis.Addr != "" {
		redisClient, err := store.NewRedis(ctx, cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			slog.Warn("连接 Redis 失败，日志流功能不可用", "error", err)
		} else {
			rdb = redisClient.Client
			logStream = logstream.NewLogStream(rdb)
			defer redisClient.Close()
		}
	} else {
		slog.Info("Redis 未配置，日志流功能不可用")
	}

	// 连接 etcd（可选，用于 Leader Election 和节点存活检测）
	var etcdClient *clientv3.Client
	if len(cfg.Etcd.Endpoints) > 0 {
		etcdConnCtx, etcdCancel := context.WithTimeout(ctx, time.Duration(cfg.Etcd.DialTimeoutSec)*time.Second)
		etcdStore, err := store.NewEtcdStore(etcdConnCtx, cfg.Etcd.Endpoints, time.Duration(cfg.Etcd.DialTimeoutSec)*time.Second)
		etcdCancel()
		if err != nil {
			slog.Warn("连接 etcd 失败，Leader Election 和 etcd Lease 不可用", "error", err)
		} else {
			etcdClient = etcdStore.Client
			defer etcdStore.Close()
		}
	} else {
		slog.Info("etcd 未配置，Leader Election 和 etcd Lease 不可用")
	}

	// 创建任务队列（优先使用 Redis，否则用内存队列）
	var taskQueue queue.TaskQueue
	if rdb != nil {
		taskQueue = queue.NewRedisQueue(rdb)
		defer taskQueue.Close()
	} else {
		taskQueue = queue.NewInMemQueue(100)
		defer taskQueue.Close()
	}

	// 初始化各层
	taskStore := store.NewTaskStore(pg)
	taskSvc := service.NewTaskService(taskStore, rdb, taskQueue)
	exec := executor.NewExecutor(cfg.Scheduler.MaxConcurrentTasks, logStream)

	// 初始化节点存储和管理器
	nodeStore := store.NewNodeStore(pg)
	nodeMgr := nodemanager.NewNodeManager(nodeStore, etcdClient)

	// 启动节点离线检查
	nodeCtx, nodeCancel := context.WithCancel(context.Background())
	defer nodeCancel()
	go nodeMgr.StartOfflineChecker(nodeCtx)

	// 启动 etcd 节点监控（当 etcd 可用时）
	if etcdClient != nil {
		nodeWatcherCtx, nodeWatcherCancel := context.WithCancel(context.Background())
		defer nodeWatcherCancel()
		nodeMgr.StartNodeWatcher(nodeWatcherCtx)
	}

	sched := scheduler.NewScheduler(
		taskStore,
		exec,
		time.Duration(cfg.Scheduler.TickIntervalMs)*time.Millisecond,
		cfg.Scheduler.MaxConcurrentTasks,
		rdb,
		taskQueue,
	)
	sched.SetNodeManager(nodeMgr)

	// 启动调度器
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	go sched.Start(schedCtx)

	// 设置 Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(middleware.RateLimit(rdb, cfg.RateLimit.Enabled, cfg.RateLimit.RequestsPerMinute))
	api.RegisterRoutes(router, taskSvc, sched, logStream, rdb, nodeMgr)

	// 启动 HTTP Server
	srv := &http.Server{
		Addr:    cfg.Server.Addr(),
		Handler: router,
	}

	go func() {
		slog.Info("服务启动中", "addr", cfg.Server.Addr())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务错误", "error", err)
			os.Exit(1)
		}
	}()

	// 启动 gRPC Server
	grpcSrv := grpc.NewServer()
	agentSrv := grpcserver.NewAgentServer(nodeMgr, sched)
	grpcserver.RegisterWithGRPC(grpcSrv, agentSrv)

	go func() {
		lis, err := net.Listen("tcp", ":50051")
		if err != nil {
			slog.Error("gRPC 监听失败", "error", err)
			return
		}
		slog.Info("gRPC 服务启动中", "addr", ":50051")
		if err := grpcSrv.Serve(lis); err != nil {
			slog.Error("gRPC 服务错误", "error", err)
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	slog.Info("收到信号，正在关闭", "signal", sig)

	// 优雅关闭：HTTP 优先，然后 gRPC，再停调度器，数据库连接由 defer 处理
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	slog.Info("正在关闭 HTTP 服务")
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP 服务强制关闭", "error", err)
	}

	slog.Info("正在关闭 gRPC 服务")
	grpcSrv.GracefulStop()

	slog.Info("正在关闭调度器")
	schedCancel()

	slog.Info("服务已退出")
}

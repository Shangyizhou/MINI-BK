package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用全局配置。
type Config struct {
	Server    ServerConfig    `mapstructure:"server"`
	Database  DatabaseConfig  `mapstructure:"database"`
	Redis     RedisConfig     `mapstructure:"redis"`
	Scheduler SchedulerConfig `mapstructure:"scheduler"`
	Executor  ExecutorConfig  `mapstructure:"executor"`
	Retry     RetryConfig     `mapstructure:"retry"`
	RateLimit RateLimitConfig `mapstructure:"rate_limit"`
	GRPC      GRPCConfig      `mapstructure:"grpc"`
	Agent     AgentConfig     `mapstructure:"agent"`
}

// RedisConfig Redis 连接配置。
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// RetryConfig 重试策略配置。
type RetryConfig struct {
	MaxAttempts        int `mapstructure:"max_attempts"`
	InitialIntervalSec int `mapstructure:"initial_interval_sec"`
	MaxIntervalSec     int `mapstructure:"max_interval_sec"`
	Multiplier         int `mapstructure:"multiplier"`
}

// RateLimitConfig 限流配置。
type RateLimitConfig struct {
	Enabled           bool `mapstructure:"enabled"`
	RequestsPerMinute int  `mapstructure:"requests_per_minute"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Host string `mapstructure:"host"`
}

// Addr 返回监听地址。
func (s ServerConfig) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// DatabaseConfig 数据库连接配置。
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
}

// DSN 返回 PostgreSQL 连接字符串。
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// SchedulerConfig 调度器配置。
type SchedulerConfig struct {
	TickIntervalMs     int `mapstructure:"tick_interval_ms"`
	MaxConcurrentTasks int `mapstructure:"max_concurrent_tasks"`
}

// ExecutorConfig 执行器配置。
type ExecutorConfig struct {
	DefaultTimeoutSec int    `mapstructure:"default_timeout_sec"`
	DefaultWorkdir    string `mapstructure:"default_workdir"`
}

// GRPCConfig gRPC 服务配置。
type GRPCConfig struct {
	Port int `mapstructure:"port"`
}

// AgentConfig Agent 心跳配置。
type AgentConfig struct {
	HeartbeatIntervalSec int `mapstructure:"heartbeat_interval_sec"`
	HeartbeatTimeoutSec  int `mapstructure:"heartbeat_timeout_sec"`
}

// Load 从指定路径加载配置，支持环境变量覆盖（MINIBK_ 前缀）。
func Load(configPath string) (*Config, error) {
	v := viper.New()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	// 环境变量覆盖: MINIBK_SERVER_PORT → server.port
	v.SetEnvPrefix("MINIBK")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 设置默认值
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.sslmode", "disable")
	v.SetDefault("redis.addr", "localhost:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)
	v.SetDefault("scheduler.tick_interval_ms", 500)
	v.SetDefault("scheduler.max_concurrent_tasks", 10)
	v.SetDefault("executor.default_timeout_sec", 300)
	v.SetDefault("executor.default_workdir", "/tmp")
	v.SetDefault("retry.max_attempts", 3)
	v.SetDefault("retry.initial_interval_sec", 1)
	v.SetDefault("retry.max_interval_sec", 60)
	v.SetDefault("retry.multiplier", 2)
	v.SetDefault("rate_limit.enabled", true)
	v.SetDefault("rate_limit.requests_per_minute", 100)
	v.SetDefault("grpc.port", 50051)
	v.SetDefault("agent.heartbeat_interval_sec", 10)
	v.SetDefault("agent.heartbeat_timeout_sec", 30)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// 配置文件不存在时可以接受，使用默认值+环境变量
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// 环境变量手动覆盖（Viper 的 AutomaticEnv + Unmarshal 有已知限制）
	if port := os.Getenv("MINIBK_SERVER_PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			slog.Warn("invalid MINIBK_SERVER_PORT, using default", "value", port, "error", err)
		} else {
			cfg.Server.Port = p
		}
	}
	if host := os.Getenv("MINIBK_DATABASE_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if port := os.Getenv("MINIBK_DATABASE_PORT"); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			slog.Warn("invalid MINIBK_DATABASE_PORT, using default", "value", port, "error", err)
		} else {
			cfg.Database.Port = p
		}
	}
	if user := os.Getenv("MINIBK_DATABASE_USER"); user != "" {
		cfg.Database.User = user
	}
	if password := os.Getenv("MINIBK_DATABASE_PASSWORD"); password != "" {
		cfg.Database.Password = password
	}
	if dbname := os.Getenv("MINIBK_DATABASE_DBNAME"); dbname != "" {
		cfg.Database.DBName = dbname
	}
	if sslmode := os.Getenv("MINIBK_DATABASE_SSLMODE"); sslmode != "" {
		cfg.Database.SSLMode = sslmode
	}
	if tick := os.Getenv("MINIBK_SCHEDULER_TICK_INTERVAL_MS"); tick != "" {
		t, err := strconv.Atoi(tick)
		if err != nil {
			slog.Warn("invalid MINIBK_SCHEDULER_TICK_INTERVAL_MS, using default", "value", tick, "error", err)
		} else {
			cfg.Scheduler.TickIntervalMs = t
		}
	}
	if max := os.Getenv("MINIBK_SCHEDULER_MAX_CONCURRENT_TASKS"); max != "" {
		m, err := strconv.Atoi(max)
		if err != nil {
			slog.Warn("invalid MINIBK_SCHEDULER_MAX_CONCURRENT_TASKS, using default", "value", max, "error", err)
		} else {
			cfg.Scheduler.MaxConcurrentTasks = m
		}
	}
	if timeout := os.Getenv("MINIBK_EXECUTOR_DEFAULT_TIMEOUT_SEC"); timeout != "" {
		t, err := strconv.Atoi(timeout)
		if err != nil {
			slog.Warn("invalid MINIBK_EXECUTOR_DEFAULT_TIMEOUT_SEC, using default", "value", timeout, "error", err)
		} else {
			cfg.Executor.DefaultTimeoutSec = t
		}
	}
	if workdir := os.Getenv("MINIBK_EXECUTOR_DEFAULT_WORKDIR"); workdir != "" {
		cfg.Executor.DefaultWorkdir = workdir
	}

	return &cfg, nil
}

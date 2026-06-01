package config

import (
    "fmt"
    "os"
    "strings"

    "github.com/spf13/viper"
)

// Config 应用全局配置。
type Config struct {
    Server    ServerConfig    `mapstructure:"server"`
    Database  DatabaseConfig  `mapstructure:"database"`
    Scheduler SchedulerConfig `mapstructure:"scheduler"`
    Executor  ExecutorConfig  `mapstructure:"executor"`
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
    v.SetDefault("scheduler.tick_interval_ms", 500)
    v.SetDefault("scheduler.max_concurrent_tasks", 10)
    v.SetDefault("executor.default_timeout_sec", 300)
    v.SetDefault("executor.default_workdir", "/tmp")

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
        fmt.Sscanf(port, "%d", &cfg.Server.Port)
    }
    if host := os.Getenv("MINIBK_DATABASE_HOST"); host != "" {
        cfg.Database.Host = host
    }

    return &cfg, nil
}

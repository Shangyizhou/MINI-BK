package config

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoadConfig(t *testing.T) {
    // 创建临时配置文件
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.yaml")
    content := `
server:
  port: 9090
database:
  host: "testhost"
  port: 5433
  user: "testuser"
  password: "testpass"
  dbname: "testdb"
  sslmode: "disable"
scheduler:
  tick_interval_ms: 1000
  max_concurrent_tasks: 5
executor:
  default_timeout_sec: 60
  default_workdir: "/var/tmp"
`
    if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
        t.Fatal(err)
    }

    cfg, err := Load(configPath)
    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
    if cfg.Server.Port != 9090 {
        t.Errorf("Server.Port = %d, want 9090", cfg.Server.Port)
    }
    if cfg.Database.Host != "testhost" {
        t.Errorf("Database.Host = %s, want testhost", cfg.Database.Host)
    }
    if cfg.Scheduler.MaxConcurrentTasks != 5 {
        t.Errorf("Scheduler.MaxConcurrentTasks = %d, want 5", cfg.Scheduler.MaxConcurrentTasks)
    }
    if cfg.Executor.DefaultWorkdir != "/var/tmp" {
        t.Errorf("Executor.DefaultWorkdir = %s, want /var/tmp", cfg.Executor.DefaultWorkdir)
    }
}

func TestLoadConfigEnvOverride(t *testing.T) {
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "config.yaml")
    content := `
server:
  port: 8080
database:
  host: "localhost"
  port: 5432
  user: "dbuser"
  password: "dbpass"
  dbname: "testdb"
  sslmode: "disable"
scheduler:
  tick_interval_ms: 500
  max_concurrent_tasks: 10
executor:
  default_timeout_sec: 300
  default_workdir: "/tmp"
`
    os.WriteFile(configPath, []byte(content), 0644)
    os.Setenv("MINIBK_SERVER_PORT", "7070")
    os.Setenv("MINIBK_DATABASE_HOST", "override-host")
    defer os.Unsetenv("MINIBK_SERVER_PORT")
    defer os.Unsetenv("MINIBK_DATABASE_HOST")

    cfg, err := Load(configPath)
    if err != nil {
        t.Fatalf("Load() error = %v", err)
    }
    if cfg.Server.Port != 7070 {
        t.Errorf("Server.Port = %d, want 7070 (env override)", cfg.Server.Port)
    }
    if cfg.Database.Host != "override-host" {
        t.Errorf("Database.Host = %s, want override-host (env override)", cfg.Database.Host)
    }
}

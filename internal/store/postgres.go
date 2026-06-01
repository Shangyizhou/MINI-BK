package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/lib/pq"
)

// Postgres 封装 PostgreSQL 数据库连接。
type Postgres struct {
	DB *sql.DB
}

// NewPostgres 创建 PostgreSQL 连接并验证连通性。
func NewPostgres(ctx context.Context, dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	slog.Info("已连接到 PostgreSQL")
	return &Postgres{DB: db}, nil
}

// Close 关闭数据库连接。
func (p *Postgres) Close() error {
	return p.DB.Close()
}

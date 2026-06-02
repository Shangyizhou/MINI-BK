package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// Redis 封装 Redis 客户端连接。
type Redis struct {
	Client *redis.Client
}

// NewRedis 创建并测试 Redis 连接。
func NewRedis(ctx context.Context, addr, password string, db int) (*Redis, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	slog.Info("已连接到 Redis")
	return &Redis{Client: rdb}, nil
}

// Close 关闭 Redis 连接。
func (r *Redis) Close() error {
	return r.Client.Close()
}

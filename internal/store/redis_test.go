package store

import (
	"context"
	"testing"
	"time"
)

func TestNewRedis(t *testing.T) {
	rdb, err := NewRedis(context.Background(), "localhost:6379", "", 0)
	if err != nil {
		t.Skipf("跳过：无法连接 Redis: %v", err)
	}
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping 失败: %v", err)
	}
}

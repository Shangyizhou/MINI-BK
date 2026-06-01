package store

import (
	"context"
	"testing"
	"time"
)

func TestNewPostgres(t *testing.T) {
	dsn := "postgres://mini-bk:mini-bk@localhost:5432/mini-bk?sslmode=disable"
	db, err := NewPostgres(context.Background(), dsn)
	if err != nil {
		t.Skipf("跳过集成测试：无法连接 PostgreSQL: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := db.DB.PingContext(ctx); err != nil {
		t.Fatalf("ping 失败: %v", err)
	}
}

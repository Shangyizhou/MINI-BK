package queue

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shangyizhou/mini-bk/internal/model"
)

func setupRedisQueue(t *testing.T) (*RedisQueue, func()) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("跳过：无法连接 Redis: %v", err)
	}

	// 清空所有数据
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("FlushDB error: %v", err)
	}

	q := NewRedisQueue(rdb)
	return q, func() {
		q.Close()
	}
}

func TestRedisQueue_PushPop(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx := context.Background()
	task := model.NewTask("test", "echo hello")

	if err := q.Push(ctx, task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	got, err := q.Pop(ctx, time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if got == nil {
		t.Fatal("Pop() 返回 nil，期望有任务")
	}
	if got.TaskUID != task.TaskUID {
		t.Errorf("Pop() TaskUID = %s, 期望 %s", got.TaskUID, task.TaskUID)
	}
	if got.Name != task.Name {
		t.Errorf("Pop() Name = %s, 期望 %s", got.Name, task.Name)
	}
}

func TestRedisQueue_PopTimeout(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx := context.Background()

	got, err := q.Pop(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if got != nil {
		t.Error("Pop() 应该返回 nil（超时）")
	}
}

func TestRedisQueue_PriorityOrdering(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx := context.Background()

	// 创建低优先级任务先入队
	low := model.NewTask("low", "echo low")
	low.Priority = 1
	if err := q.PushPriority(ctx, low); err != nil {
		t.Fatalf("PushPriority() error = %v", err)
	}

	// 创建高优先级任务后入队
	high := model.NewTask("high", "echo high")
	high.Priority = 10
	if err := q.PushPriority(ctx, high); err != nil {
		t.Fatalf("PushPriority() error = %v", err)
	}

	// 高优先级（priority=10）应最先出队（score = -10 < -1）
	first, err := q.Pop(ctx, time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if first == nil {
		t.Fatal("第一个 Pop 返回 nil")
	}
	if first.Name != "high" {
		t.Errorf("期望先取出 high 优先级任务，实际取出了 %s", first.Name)
	}

	second, err := q.Pop(ctx, time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if second == nil {
		t.Fatal("第二个 Pop 返回 nil")
	}
	if second.Name != "low" {
		t.Errorf("期望第二个取出 low 优先级任务，实际取出了 %s", second.Name)
	}
}

func TestRedisQueue_PushPopPriorityThenNormal(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx := context.Background()

	// 先入队一个普通任务
	normal := model.NewTask("normal", "echo normal")
	if err := q.Push(ctx, normal); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// 再入队一个优先级任务（应优先出队）
	high := model.NewTask("high", "echo high")
	high.Priority = 5
	if err := q.PushPriority(ctx, high); err != nil {
		t.Fatalf("PushPriority() error = %v", err)
	}

	// 优先级任务应优先出队
	first, err := q.Pop(ctx, time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if first == nil {
		t.Fatal("第一个 Pop 返回 nil")
	}
	if first.Name != "high" {
		t.Errorf("期望先取出 high 优先级任务，实际取出了 %s", first.Name)
	}

	// 然后取出普通任务
	second, err := q.Pop(ctx, time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if second == nil {
		t.Fatal("第二个 Pop 返回 nil")
	}
	if second.Name != "normal" {
		t.Errorf("期望第二个取出 normal 任务，实际取出了 %s", second.Name)
	}
}

func TestRedisQueue_PushDelayed(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx := context.Background()
	task := model.NewTask("delayed", "echo delayed")

	if err := q.PushDelayed(ctx, task, 2*time.Second); err != nil {
		t.Fatalf("PushDelayed() error = %v", err)
	}

	// 未到期时不应出队
	immediate, err := q.Pop(ctx, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if immediate != nil {
		t.Error("延迟任务在到期前不应出队")
	}

	// 等待延迟到期（processDelayedLoop 每 500ms 运行一次 + 2s delay）
	got, err := q.Pop(ctx, 5*time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if got == nil {
		t.Fatal("延迟任务到期后应能出队")
	}
	if got.Name != "delayed" {
		t.Errorf("期望取出 delayed 任务，实际取出了 %s", got.Name)
	}
	if got.TaskUID != task.TaskUID {
		t.Errorf("TaskUID = %s, 期望 %s", got.TaskUID, task.TaskUID)
	}
}

func TestRedisQueue_Size(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx := context.Background()

	size, err := q.Size(ctx)
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}
	if size != 0 {
		t.Errorf("Size() = %d, 期望 0", size)
	}

	// 入队 3 个普通任务
	for i := 0; i < 3; i++ {
		task := model.NewTask("normal", "echo normal")
		if err := q.Push(ctx, task); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
	}

	// 入队 2 个优先级任务
	for i := 0; i < 2; i++ {
		task := model.NewTask("priority", "echo priority")
		task.Priority = 5
		if err := q.PushPriority(ctx, task); err != nil {
			t.Fatalf("PushPriority() error = %v", err)
		}
	}

	// 入队 1 个延迟任务
	task := model.NewTask("delayed", "echo delayed")
	if err := q.PushDelayed(ctx, task, time.Minute); err != nil {
		t.Fatalf("PushDelayed() error = %v", err)
	}

	size, err = q.Size(ctx)
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}
	if size != 6 {
		t.Errorf("Size() = %d, 期望 6", size)
	}
}

func TestRedisQueue_Ack(t *testing.T) {
	q, cleanup := setupRedisQueue(t)
	defer cleanup()

	ctx := context.Background()
	task := model.NewTask("ack-test", "echo test")
	if err := q.Push(ctx, task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// Ack 应无错误（Redis 实现为 no-op）
	if err := q.Ack(ctx, task.TaskUID); err != nil {
		t.Errorf("Ack() error = %v", err)
	}
}

func TestRedisQueue_Close(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("跳过：无法连接 Redis: %v", err)
	}

	q := NewRedisQueue(rdb)

	if err := q.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 再次关闭应无 panic
	if err := q.Close(); err != nil {
		t.Errorf("第二次 Close() error = %v", err)
	}
}

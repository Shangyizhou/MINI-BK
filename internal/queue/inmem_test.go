package queue

import (
	"context"
	"testing"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
)

func TestInMemQueue_PushPop(t *testing.T) {
	q := NewInMemQueue(10)
	defer q.Close()

	task := model.NewTask("test", "echo hello")

	if err := q.Push(context.Background(), task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	got, err := q.Pop(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if got == nil {
		t.Fatal("Pop() 返回 nil，期望有任务")
	}
	if got.TaskUID != task.TaskUID {
		t.Errorf("Pop() TaskUID = %s, 期望 %s", got.TaskUID, task.TaskUID)
	}
}

func TestInMemQueue_Size(t *testing.T) {
	q := NewInMemQueue(10)
	defer q.Close()

	ctx := context.Background()

	size, err := q.Size(ctx)
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}
	if size != 0 {
		t.Errorf("Size() = %d, 期望 0", size)
	}

	for i := 0; i < 5; i++ {
		task := model.NewTask("test", "echo hello")
		if err := q.Push(ctx, task); err != nil {
			t.Fatalf("Push() error = %v", err)
		}
	}

	size, err = q.Size(ctx)
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}
	if size != 5 {
		t.Errorf("Size() = %d, 期望 5", size)
	}

	// Pop one
	task, err := q.Pop(ctx, time.Second)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if task == nil {
		t.Fatal("Pop() 返回 nil")
	}

	size, _ = q.Size(ctx)
	if size != 4 {
		t.Errorf("Size() = %d, 期望 4", size)
	}
}

func TestInMemQueue_PopTimeout(t *testing.T) {
	q := NewInMemQueue(10)
	defer q.Close()

	task, err := q.Pop(context.Background(), 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if task != nil {
		t.Error("Pop() 应该返回 nil（超时）")
	}
}

func TestInMemQueue_PushPriority(t *testing.T) {
	q := NewInMemQueue(10)
	defer q.Close()

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

	// 创建普通任务
	normal := model.NewTask("normal", "echo normal")
	if err := q.Push(ctx, normal); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// 因为 priority 通过 goroutine 搬移到 pending channel,
	// 需要等待短暂时间让搬移完成
	time.Sleep(100 * time.Millisecond)

	// 验证优先级 (drain 会先搬移到 pending)
	// 高优先级任务应最先被消费
	first, _ := q.Pop(ctx, time.Second)
	if first == nil {
		t.Fatal("第一个 Pop 返回 nil")
	}
	if first.Name != "high" {
		t.Errorf("期望先取出 high 优先级任务，实际取出了 %s", first.Name)
	}

	second, _ := q.Pop(ctx, time.Second)
	if second == nil {
		t.Fatal("第二个 Pop 返回 nil")
	}
	if second.Name != "low" {
		t.Errorf("期望第二个取出 low 优先级任务，实际取出了 %s", second.Name)
	}

	third, _ := q.Pop(ctx, time.Second)
	if third == nil {
		t.Fatal("第三个 Pop 返回 nil")
	}
	if third.Name != "normal" {
		t.Errorf("期望第三个取出 normal 任务，实际取出了 %s", third.Name)
	}
}

func TestInMemQueue_PushDelayed(t *testing.T) {
	q := NewInMemQueue(10)
	defer q.Close()

	ctx := context.Background()

	task := model.NewTask("delayed", "echo delayed")

	if err := q.PushDelayed(ctx, task, 100*time.Millisecond); err != nil {
		t.Fatalf("PushDelayed() error = %v", err)
	}

	// 未到期时不应出队
	immediate, err := q.Pop(ctx, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if immediate != nil {
		t.Error("延迟任务在到期前不应出队")
	}

	// 等待延迟到期
	got, err := q.Pop(ctx, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Pop() error = %v", err)
	}
	if got == nil {
		t.Fatal("延迟任务到期后应能出队")
	}
	if got.TaskUID != task.TaskUID {
		t.Errorf("TaskUID = %s, 期望 %s", got.TaskUID, task.TaskUID)
	}
}

func TestInMemQueue_Ack(t *testing.T) {
	q := NewInMemQueue(10)
	defer q.Close()

	ctx := context.Background()
	task := model.NewTask("ack-test", "echo test")
	if err := q.Push(ctx, task); err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// Ack 应无错误（InMem 为 no-op）
	if err := q.Ack(ctx, task.TaskUID); err != nil {
		t.Errorf("Ack() error = %v", err)
	}
}

func TestInMemQueue_ConcurrentPushPop(t *testing.T) {
	q := NewInMemQueue(100)
	defer q.Close()

	ctx := context.Background()
	numTasks := 50

	// 并发入队
	done := make(chan struct{})
	go func() {
		for i := 0; i < numTasks; i++ {
			task := model.NewTask("concurrent", "echo hello")
			q.Push(ctx, task)
		}
		close(done)
	}()

	// 边入队边出队
	consumed := 0
	for consumed < numTasks {
		task, err := q.Pop(ctx, 5*time.Second)
		if err != nil {
			t.Fatalf("Pop() error = %v", err)
		}
		if task != nil {
			consumed++
		}
	}

	<-done

	if consumed != numTasks {
		t.Errorf("消费了 %d 个任务，期望 %d", consumed, numTasks)
	}
}

func TestInMemQueue_SizeWithPriorityAndDelayed(t *testing.T) {
	q := NewInMemQueue(10)
	defer q.Close()

	ctx := context.Background()

	// 普通任务
	q.Push(ctx, model.NewTask("n1", "echo"))
	q.Push(ctx, model.NewTask("n2", "echo"))

	// 优先级任务
	q.PushPriority(ctx, model.NewTask("p1", "echo"))

	// 延迟任务
	q.PushDelayed(ctx, model.NewTask("d1", "echo"), time.Minute)

	size, err := q.Size(ctx)
	if err != nil {
		t.Fatalf("Size() error = %v", err)
	}
	// 2 normal + (priority 已搬移到 pending) + 1 delayed
	// 注意：priority 可能在 drain goroutine 中已搬移到 pending,
	// 所以 size 至少为 3
	if size < 3 {
		t.Errorf("Size() = %d, 期望至少 3", size)
	}
}

func TestInMemQueue_Close(t *testing.T) {
	q := NewInMemQueue(10)

	// 入队一些任务
	for i := 0; i < 5; i++ {
		q.Push(context.Background(), model.NewTask("close-test", "echo"))
	}

	// 关闭不应阻塞
	if err := q.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 再次关闭应无问题
	if err := q.Close(); err != nil {
		t.Errorf("第二次 Close() error = %v", err)
	}
}

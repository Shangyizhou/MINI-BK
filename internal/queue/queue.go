package queue

import (
	"context"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
)

// TaskQueue 定义任务队列的抽象接口。
type TaskQueue interface {
	// Push 将任务添加到普通队列尾部。
	Push(ctx context.Context, task *model.Task) error
	// Pop 从队列头部取出一个任务，超时返回 nil。
	Pop(ctx context.Context, timeout time.Duration) (*model.Task, error)
	// PushPriority 将任务按优先级插入（优先级越高越先出队）。
	PushPriority(ctx context.Context, task *model.Task) error
	// PushDelayed 将任务延迟指定时间后入队。
	PushDelayed(ctx context.Context, task *model.Task, delay time.Duration) error
	// Ack 确认任务处理完成。
	Ack(ctx context.Context, taskUID string) error
	// Size 返回队列中待处理的任务数量。
	Size(ctx context.Context) (int64, error)
	// Close 关闭队列，释放资源。
	Close() error
}

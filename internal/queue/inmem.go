package queue

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/shangyizhou/mini-bk/internal/model"
)

// delayedItem 存储延迟任务及其到期时间。
type delayedItem struct {
	task *model.Task
	due  time.Time
}

// priorityQueue 实现 container/heap.Interface，按 Priority 降序排列。
type priorityQueue []*model.Task

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	// 优先级越高（值越大）越先出队
	return pq[i].Priority > pq[j].Priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *priorityQueue) Push(x any) {
	*pq = append(*pq, x.(*model.Task))
}

func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // 避免内存泄漏
	*pq = old[:n-1]
	return item
}

// InMemQueue 是一个内存中的任务队列实现。
// 使用共享缓冲区 + 互斥锁实现线程安全，
// 支持普通队列（FIFO）、优先级队列（最高优先）和延迟任务。
type InMemQueue struct {
	mu      sync.Mutex
	items   []*model.Task // 普通任务队列（FIFO）
	pq      priorityQueue // 优先级队列（最高优先）
	closed  bool
	stopCh  chan struct{}
	wg      sync.WaitGroup

	delayed   []delayedItem
	delayedMu sync.Mutex
}

// NewInMemQueue 创建一个新的内存队列。
func NewInMemQueue(capacity int) *InMemQueue {
	q := &InMemQueue{
		items:  make([]*model.Task, 0, capacity),
		stopCh: make(chan struct{}),
	}

	// 后台 goroutine：检查延迟任务是否到期
	q.wg.Add(1)
	go q.drainDelayedLoop()

	return q
}

// drainDelayedLoop 持续检查延迟任务是否到期，到期则移入普通队列。
func (q *InMemQueue) drainDelayedLoop() {
	defer q.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			q.delayedMu.Lock()
			var remaining []delayedItem
			for _, d := range q.delayed {
				if now.After(d.due) || now.Equal(d.due) {
					q.mu.Lock()
					q.items = append(q.items, d.task)
					q.mu.Unlock()
				} else {
					remaining = append(remaining, d)
				}
			}
			q.delayed = remaining
			q.delayedMu.Unlock()
		}
	}
}

// popInternal 内部取出一个任务，调用者必须持有 q.mu。
// 先从优先级队列取，再从普通队列取。
func (q *InMemQueue) popInternal() *model.Task {
	if q.pq.Len() > 0 {
		return heap.Pop(&q.pq).(*model.Task)
	}
	if len(q.items) > 0 {
		task := q.items[0]
		q.items = q.items[1:]
		return task
	}
	return nil
}

// Push 将任务添加到普通队列尾部。
func (q *InMemQueue) Push(_ context.Context, task *model.Task) error {
	q.mu.Lock()
	q.items = append(q.items, task)
	q.mu.Unlock()
	return nil
}

// Pop 从队列中取出一个任务。优先级队列中的任务会优先于普通任务出队。
// timeout 超时后返回 nil, nil。
func (q *InMemQueue) Pop(_ context.Context, timeout time.Duration) (*model.Task, error) {
	deadline := time.Now().Add(timeout)

	for {
		q.mu.Lock()
		if task := q.popInternal(); task != nil {
			q.mu.Unlock()
			return task, nil
		}
		q.mu.Unlock()

		if time.Now().After(deadline) {
			return nil, nil
		}

		remaining := time.Until(deadline)
		sleep := remaining
		if sleep > 10*time.Millisecond {
			sleep = 10 * time.Millisecond
		}
		time.Sleep(sleep)
	}
}

// PushPriority 将任务按优先级插入（优先级越高越先出队）。
func (q *InMemQueue) PushPriority(_ context.Context, task *model.Task) error {
	q.mu.Lock()
	heap.Push(&q.pq, task)
	q.mu.Unlock()
	return nil
}

// PushDelayed 将任务延迟指定时间后入队。
func (q *InMemQueue) PushDelayed(_ context.Context, task *model.Task, delay time.Duration) error {
	q.delayedMu.Lock()
	q.delayed = append(q.delayed, delayedItem{
		task: task,
		due:  time.Now().Add(delay),
	})
	q.delayedMu.Unlock()
	return nil
}

// Ack 确认任务处理完成（内存实现中为 no-op）。
func (q *InMemQueue) Ack(_ context.Context, _ string) error {
	return nil
}

// Size 返回队列中待处理的任务数量（包括尚未到期的延迟任务）。
func (q *InMemQueue) Size(_ context.Context) (int64, error) {
	q.mu.Lock()
	total := len(q.items) + q.pq.Len()
	q.mu.Unlock()

	q.delayedMu.Lock()
	total += len(q.delayed)
	q.delayedMu.Unlock()

	return int64(total), nil
}

// Close 关闭队列，释放资源。
func (q *InMemQueue) Close() error {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil
	}
	q.closed = true
	close(q.stopCh)
	q.mu.Unlock()

	q.wg.Wait()
	return nil
}

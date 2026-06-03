package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shangyizhou/mini-bk/internal/model"
)

const (
	redisTaskKey    = "tasks:task:%s"
	redisPendingKey = "tasks:queue:pending"
	redisPriorityKey = "tasks:queue:priority"
	redisDelayedKey = "tasks:queue:delayed"
	taskTTL         = 24 * time.Hour
)

// RedisQueue 是基于 Redis 的任务队列实现。
type RedisQueue struct {
	rdb    *redis.Client
	closed atomic.Bool
	stopCh chan struct{}
}

// NewRedisQueue 创建一个新的 Redis 队列。
// 同时启动后台 goroutine 定期将到期的延迟任务移至待处理队列。
func NewRedisQueue(rdb *redis.Client) *RedisQueue {
	q := &RedisQueue{
		rdb:    rdb,
		stopCh: make(chan struct{}),
	}
	go q.processDelayedLoop()
	return q
}

// processDelayedLoop 持续检查延迟任务是否到期，到期则移入 pending 队列。
func (q *RedisQueue) processDelayedLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-q.stopCh:
			return
		case <-ticker.C:
			q.moveDueDelayed()
		}
	}
}

// moveDueDelayed 将所有到期的延迟任务移至 pending 队列。
func (q *RedisQueue) moveDueDelayed() {
	ctx := context.Background()
	now := time.Now().Unix()

	// 获取所有已到期延迟任务的 UID
	taskUIDs, err := q.rdb.ZRangeByScore(ctx, redisDelayedKey, &redis.ZRangeBy{
		Min: "0",
		Max: strconv.FormatInt(now, 10),
	}).Result()
	if err != nil {
		slog.Error("redis_queue: failed to query delayed tasks", "error", err)
		return
	}

	for _, uid := range taskUIDs {
		// 移到 pending 队列
		if err := q.rdb.LPush(ctx, redisPendingKey, uid).Err(); err != nil {
			slog.Error("redis_queue: failed to move delayed task to pending", "task_uid", uid, "error", err)
			continue
		}
		// 从延迟队列移除
		if err := q.rdb.ZRem(ctx, redisDelayedKey, uid).Err(); err != nil {
			slog.Error("redis_queue: failed to remove delayed task", "task_uid", uid, "error", err)
		}
	}
}

// storeTask 将任务数据序列化写入 Redis，并设置 TTL。
func (q *RedisQueue) storeTask(ctx context.Context, task *model.Task) error {
	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	return q.rdb.SetEx(ctx, fmt.Sprintf(redisTaskKey, task.TaskUID), data, taskTTL).Err()
}

// getTask 从 Redis 读取并反序列化任务数据。
func (q *RedisQueue) getTask(ctx context.Context, uid string) (*model.Task, error) {
	data, err := q.rdb.Get(ctx, fmt.Sprintf(redisTaskKey, uid)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}
	var task model.Task
	if err := json.Unmarshal([]byte(data), &task); err != nil {
		return nil, fmt.Errorf("unmarshal task: %w", err)
	}
	return &task, nil
}

// Push 将任务添加到普通队列尾部。
func (q *RedisQueue) Push(ctx context.Context, task *model.Task) error {
	if err := q.storeTask(ctx, task); err != nil {
		return err
	}
	return q.rdb.LPush(ctx, redisPendingKey, task.TaskUID).Err()
}

// Pop 从队列中取出一个任务。优先处理优先级队列中的任务。
// 超时则返回 nil, nil。
func (q *RedisQueue) Pop(ctx context.Context, timeout time.Duration) (*model.Task, error) {
	// 先检查优先级队列
	results, err := q.rdb.ZPopMin(ctx, redisPriorityKey, 1).Result()
	if err != nil {
		return nil, err
	}
	if len(results) > 0 {
		uid := results[0].Member.(string)
		task, err := q.getTask(ctx, uid)
		if err != nil {
			return nil, err
		}
		return task, nil
	}

	// 再检查待处理队列
	result, err := q.rdb.BRPop(ctx, timeout, redisPendingKey).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// BRPop 返回 [key, value]
	uid := result[1]
	task, err := q.getTask(ctx, uid)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// PushPriority 将任务按优先级插入（优先级越高越先出队）。
// Redis Sorted Set 使用 score = -priority，使高优先级（大值）获得小 score 从而先出。
func (q *RedisQueue) PushPriority(ctx context.Context, task *model.Task) error {
	if err := q.storeTask(ctx, task); err != nil {
		return err
	}
	return q.rdb.ZAdd(ctx, redisPriorityKey, redis.Z{
		Score:  float64(-task.Priority),
		Member: task.TaskUID,
	}).Err()
}

// PushDelayed 将任务延迟指定时间后入队。
// Redis Sorted Set 使用 score = now + delay（Unix 时间戳）。
func (q *RedisQueue) PushDelayed(ctx context.Context, task *model.Task, delay time.Duration) error {
	if err := q.storeTask(ctx, task); err != nil {
		return err
	}
	score := float64(time.Now().Unix()) + delay.Seconds()
	return q.rdb.ZAdd(ctx, redisDelayedKey, redis.Z{
		Score:  score,
		Member: task.TaskUID,
	}).Err()
}

// Ack 确认任务处理完成（Redis 实现中为 no-op，任务在 Pop 时已被移除）。
func (q *RedisQueue) Ack(_ context.Context, _ string) error {
	return nil
}

// Size 返回队列中待处理的任务数量。
func (q *RedisQueue) Size(ctx context.Context) (int64, error) {
	pending, err := q.rdb.LLen(ctx, redisPendingKey).Result()
	if err != nil {
		return 0, err
	}
	priority, err := q.rdb.ZCard(ctx, redisPriorityKey).Result()
	if err != nil {
		return 0, err
	}
	delayed, err := q.rdb.ZCard(ctx, redisDelayedKey).Result()
	if err != nil {
		return 0, err
	}
	return pending + priority + delayed, nil
}

// Close 关闭 Redis 队列，停止后台 goroutine 并关闭 Redis 客户端。
func (q *RedisQueue) Close() error {
	if q.closed.CompareAndSwap(false, true) {
		close(q.stopCh)
		return q.rdb.Close()
	}
	return nil
}

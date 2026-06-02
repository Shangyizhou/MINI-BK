package logstream

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// LogEntry 表示单条日志条目。
type LogEntry struct {
	ID        string `json:"id"`
	TaskUID   string `json:"task_uid"`
	Line      string `json:"line"`
	Stream    string `json:"stream"` // "stdout" or "stderr"
	Timestamp int64  `json:"timestamp"`
}

// LogStream 基于 Redis Stream 实现任务日志的实时流式写入和读取。
type LogStream struct {
	rdb *redis.Client
}

// NewLogStream 创建一个新的日志流实例。
func NewLogStream(rdb *redis.Client) *LogStream {
	return &LogStream{rdb: rdb}
}

// streamKey 返回指定任务日志的 Redis Stream key。
func (s *LogStream) streamKey(taskUID string) string {
	return fmt.Sprintf("tasks:log:%s", taskUID)
}

// Append 将一条日志行写入任务的 Redis Stream。
func (s *LogStream) Append(ctx context.Context, taskUID, line, stream string) error {
	return s.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: s.streamKey(taskUID),
		MaxLen: 10000, // 最多保留最近 10000 行
		Values: map[string]interface{}{
			"line":      line,
			"stream":    stream,
			"timestamp": time.Now().UnixMilli(),
		},
	}).Err()
}

// Read 读取从 lastID 开始的新日志条目。
// 如果 lastID == "0"，则阻塞等待新日志（首次读取）。
// 否则阻塞最多 1000ms 等待新日志。
func (s *LogStream) Read(ctx context.Context, taskUID, lastID string, count int64) ([]LogEntry, error) {
	block := time.Duration(0)
	if lastID == "0" {
		block = 0 // 首次读取，无限阻塞
	} else {
		block = 1000 * time.Millisecond
	}

	streams, err := s.rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{s.streamKey(taskUID), lastID},
		Count:   count,
		Block:   block,
	}).Result()
	if err == redis.Nil {
		// 没有新数据
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil, nil
	}

	entries := make([]LogEntry, 0, len(streams[0].Messages))
	for _, msg := range streams[0].Messages {
		entry := LogEntry{
			ID:      msg.ID,
			TaskUID: taskUID,
		}
		if line, ok := msg.Values["line"].(string); ok {
			entry.Line = line
		}
		if stream, ok := msg.Values["stream"].(string); ok {
			entry.Stream = stream
		}
		if tsStr, ok := msg.Values["timestamp"].(string); ok {
			if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
					entry.Timestamp = ts
				}
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

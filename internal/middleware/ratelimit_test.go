package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func setupRedisForTest(t *testing.T) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		return nil // Redis 不可用
	}
	if err := rdb.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("FlushDB error: %v", err)
	}
	return rdb
}

func TestRateLimit_Disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 即使 Redis 为 nil，禁用的中间件也应正常放行
	handler := RateLimit(nil, false, 10)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	if w.Code != http.StatusOK {
		t.Errorf("期望状态 200（已禁用），实际 %d", w.Code)
	}
}

func TestRateLimit_WithinLimit(t *testing.T) {
	rdb := setupRedisForTest(t)
	if rdb == nil {
		t.Skip("跳过：无法连接 Redis")
	}
	defer rdb.Close()

	gin.SetMode(gin.TestMode)

	handler := RateLimit(rdb, true, 10)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler(c)

		if w.Code != http.StatusOK {
			t.Errorf("第 %d 次请求：期望 200，实际 %d", i+1, w.Code)
		}

		if c.Writer.Header().Get("X-RateLimit-Remaining") == "" {
			t.Errorf("第 %d 次请求：期望 X-RateLimit-Remaining header", i+1)
		}
		if c.Writer.Header().Get("X-RateLimit-Limit") != "10" {
			t.Errorf("第 %d 次请求：期望 X-RateLimit-Limit=10，实际 %s",
				i+1, c.Writer.Header().Get("X-RateLimit-Limit"))
		}
	}
}

func TestRateLimit_Exceeded(t *testing.T) {
	rdb := setupRedisForTest(t)
	if rdb == nil {
		t.Skip("跳过：无法连接 Redis")
	}
	defer rdb.Close()

	gin.SetMode(gin.TestMode)

	handler := RateLimit(rdb, true, 3)

	// 前 3 次请求应在限制内
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.168.1.2:12345"
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req

		handler(c)

		if w.Code != http.StatusOK {
			t.Errorf("第 %d 次请求：期望 200，实际 %d", i+1, w.Code)
		}
	}

	// 第 4 次请求应被限流
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.2:12345"
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("第 4 次请求：期望 429，实际 %d", w.Code)
	}

	if c.Writer.Header().Get("Retry-After") == "" {
		t.Error("期望 Retry-After header")
	}
	if c.Writer.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("期望 X-RateLimit-Remaining=0，实际 %s",
			c.Writer.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestRateLimit_DifferentIPs(t *testing.T) {
	rdb := setupRedisForTest(t)
	if rdb == nil {
		t.Skip("跳过：无法连接 Redis")
	}
	defer rdb.Close()

	gin.SetMode(gin.TestMode)

	handler := RateLimit(rdb, true, 2)

	// 两个不同的 IP 应各自独立计数
	for _, ip := range []string{"10.0.0.1:12345", "10.0.0.2:12345"} {
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = ip
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			handler(c)

			if i < 2 && w.Code != http.StatusOK {
				t.Errorf("IP %s 第 %d 次请求：期望 200，实际 %d", ip, i+1, w.Code)
			}
			if i >= 2 && w.Code != http.StatusTooManyRequests {
				t.Errorf("IP %s 第 %d 次请求：期望 429，实际 %d", ip, i+1, w.Code)
			}
		}
	}
}

func TestRateLimit_RedisError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 使用无效地址的 Redis 客户端模拟错误
	rdb := redis.NewClient(&redis.Options{
		Addr: "localhost:16379", // 无效端口
	})

	handler := RateLimit(rdb, true, 10)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler(c)

	// Redis 错误时应放行
	if w.Code != http.StatusOK {
		t.Errorf("Redis 错误时：期望 200，实际 %d", w.Code)
	}
}

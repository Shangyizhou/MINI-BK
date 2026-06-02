package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimit 创建一个基于 Redis 的接口限流中间件。
// 每个客户端 IP 每分钟最多允许 requestsPerMinute 次请求。
// 当 enabled 为 false 时，中间件直接放行。
func RateLimit(rdb *redis.Client, enabled bool, requestsPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !enabled {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		key := fmt.Sprintf("ratelimit:%s", c.ClientIP())

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			// Redis 错误时放行，不阻塞请求
			c.Next()
			return
		}

		if count == 1 {
			// 首次访问，设置过期时间
			rdb.Expire(ctx, key, time.Minute)
		}

		if count > int64(requestsPerMinute) {
			ttl := rdb.TTL(ctx, key).Val()
			c.Header("X-RateLimit-Limit", strconv.Itoa(requestsPerMinute))
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(ttl).Unix(), 10))
			c.Header("Retry-After", strconv.FormatInt(int64(ttl.Seconds()), 10))
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后重试"})
			c.Abort()
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(requestsPerMinute))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(requestsPerMinute-int(count)))
		c.Next()
	}
}

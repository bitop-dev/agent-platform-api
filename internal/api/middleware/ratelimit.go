package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RateLimiter implements a simple token bucket per-IP rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     int           // tokens per window
	window   time.Duration // window size
	cleanTTL time.Duration // evict buckets after this idle time
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

// NewRateLimiter creates a rate limiter allowing `rate` requests per `window`.
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     rate,
		window:   window,
		cleanTTL: window * 10,
	}
	go rl.cleanup()
	return rl
}

// Middleware returns a Fiber handler that enforces rate limits.
func (rl *RateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		key := c.IP()

		rl.mu.Lock()
		b, ok := rl.buckets[key]
		now := time.Now()

		if !ok || now.Sub(b.lastReset) >= rl.window {
			b = &bucket{tokens: rl.rate, lastReset: now}
			rl.buckets[key] = b
		}

		if b.tokens <= 0 {
			rl.mu.Unlock()
			c.Set("Retry-After", "1")
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded",
			})
		}

		b.tokens--
		rl.mu.Unlock()

		return c.Next()
	}
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cleanTTL)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.cleanTTL)
		for k, b := range rl.buckets {
			if b.lastReset.Before(cutoff) {
				delete(rl.buckets, k)
			}
		}
		rl.mu.Unlock()
	}
}

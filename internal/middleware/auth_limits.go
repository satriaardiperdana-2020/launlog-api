package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// AuthLimiter is per-process, bounded, and fail-closed when its key table fills.
// Every caller uses the direct TCP peer address; forwarding headers are ignored.
type AuthLimiter struct {
	mu       sync.Mutex
	entries  map[string]rateEntry
	capacity int
	limit    int
	window   time.Duration
}

type rateEntry struct {
	count int
	until time.Time
}

func NewAuthLimiter(capacity, limit int, window time.Duration) *AuthLimiter {
	return &AuthLimiter{entries: make(map[string]rateEntry), capacity: capacity, limit: limit, window: window}
}

func (l *AuthLimiter) Middleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		key := c.Path() + ":" + c.RealIP()
		now := time.Now()
		l.mu.Lock()
		entry, found := l.entries[key]
		if found && !now.Before(entry.until) {
			delete(l.entries, key)
			found = false
		}
		if !found && len(l.entries) >= l.capacity {
			for k, v := range l.entries {
				if !now.Before(v.until) {
					delete(l.entries, k)
				}
			}
		}
		if !found && len(l.entries) >= l.capacity {
			l.mu.Unlock()
			return tooMany(c, l.window)
		}
		if !found {
			entry = rateEntry{until: now.Add(l.window)}
		}
		if entry.count <= l.limit {
			entry.count++
		}
		l.entries[key] = entry
		blocked := entry.count > l.limit
		retry := time.Until(entry.until)
		l.mu.Unlock()
		if blocked {
			return tooMany(c, retry)
		}
		return next(c)
	}
}

func tooMany(c echo.Context, retry time.Duration) error {
	seconds := int(retry.Seconds()) + 1
	if seconds < 1 {
		seconds = 1
	}
	c.Response().Header().Set(echo.HeaderRetryAfter, strconv.Itoa(seconds))
	return c.JSON(http.StatusTooManyRequests, map[string]string{"code": "TOO_MANY_REQUESTS", "message": "Please try again later."})
}

func AuthBodyLimit(limit int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().ContentLength > limit {
				return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"code": "REQUEST_TOO_LARGE", "message": "Request body is too large."})
			}
			c.Request().Body = http.MaxBytesReader(c.Response(), c.Request().Body, limit)
			return next(c)
		}
	}
}

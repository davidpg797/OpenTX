package ratelimit

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

var (
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
	ErrInvalidLimit      = errors.New("invalid rate limit configuration")
)

// RateLimiter provides rate limiting functionality
type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
	AllowN(ctx context.Context, key string, n int) (bool, error)
	Reset(ctx context.Context, key string) error
	GetLimit(key string) (*Limit, error)
}

// Limit defines rate limit configuration
type Limit struct {
	Requests int           // Number of requests allowed
	Window   time.Duration // Time window
	Burst    int           // Burst capacity (optional)
}

// TokenBucketLimiter implements token bucket algorithm
type TokenBucketLimiter struct {
	redis  *redis.Client
	logger *zap.Logger

	mu     sync.RWMutex
	limits map[string]*Limit // key prefix -> limit config

	// Default limits
	defaultLimit *Limit
}

// Config for rate limiter
type Config struct {
	DefaultRequests int
	DefaultWindow   time.Duration
	DefaultBurst    int
}

// NewTokenBucketLimiter creates a new rate limiter
func NewTokenBucketLimiter(redisClient *redis.Client, config *Config, logger *zap.Logger) *TokenBucketLimiter {
	if config == nil {
		config = &Config{
			DefaultRequests: 1000,
			DefaultWindow:   time.Minute,
			DefaultBurst:    100,
		}
	}

	return &TokenBucketLimiter{
		redis:  redisClient,
		logger: logger,
		limits: make(map[string]*Limit),
		defaultLimit: &Limit{
			Requests: config.DefaultRequests,
			Window:   config.DefaultWindow,
			Burst:    config.DefaultBurst,
		},
	}
}

// SetLimit configures a rate limit for a specific key prefix
func (rl *TokenBucketLimiter) SetLimit(keyPrefix string, limit *Limit) error {
	if limit.Requests <= 0 || limit.Window <= 0 {
		return ErrInvalidLimit
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.limits[keyPrefix] = limit
	rl.logger.Info("rate limit configured",
		zap.String("key_prefix", keyPrefix),
		zap.Int("requests", limit.Requests),
		zap.Duration("window", limit.Window))

	return nil
}

// Allow checks if a request is allowed (consumes 1 token)
func (rl *TokenBucketLimiter) Allow(ctx context.Context, key string) (bool, error) {
	return rl.AllowN(ctx, key, 1)
}

// AllowN checks if N requests are allowed
func (rl *TokenBucketLimiter) AllowN(ctx context.Context, key string, n int) (bool, error) {
	limit := rl.getLimitForKey(key)

	// Use Redis for distributed rate limiting
	allowed, err := rl.allowRedis(ctx, key, n, limit)
	if err != nil {
		rl.logger.Error("rate limit check failed",
			zap.String("key", key),
			zap.Error(err))
		// Fail open - allow request if rate limiter is down
		return true, err
	}

	if !allowed {
		rl.logger.Warn("rate limit exceeded",
			zap.String("key", key),
			zap.Int("requested", n),
			zap.Int("limit", limit.Requests))
	}

	return allowed, nil
}

// allowRedis implements distributed rate limiting using Redis
func (rl *TokenBucketLimiter) allowRedis(ctx context.Context, key string, n int, limit *Limit) (bool, error) {
	// Lua script for atomic token bucket check
	script := `
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local window = tonumber(ARGV[2])
		local requested = tonumber(ARGV[3])
		local now = tonumber(ARGV[4])

		-- Get current bucket state
		local bucket = redis.call('HMGET', key, 'tokens', 'last_update')
		local tokens = tonumber(bucket[1])
		local last_update = tonumber(bucket[2])

		if tokens == nil then
			tokens = capacity
			last_update = now
		end

		-- Calculate tokens to add based on elapsed time
		local elapsed = now - last_update
		local rate = capacity / window
		local tokens_to_add = elapsed * rate
		tokens = math.min(capacity, tokens + tokens_to_add)

		-- Check if we have enough tokens
		if tokens >= requested then
			tokens = tokens - requested
			redis.call('HMSET', key, 'tokens', tokens, 'last_update', now)
			redis.call('EXPIRE', key, window)
			return 1
		end

		return 0
	`

	result, err := rl.redis.Eval(ctx, script, []string{key},
		limit.Requests,
		limit.Window.Seconds(),
		n,
		float64(time.Now().UnixNano())/1e9,
	).Int()

	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// Reset resets the rate limit for a key
func (rl *TokenBucketLimiter) Reset(ctx context.Context, key string) error {
	return rl.redis.Del(ctx, key).Err()
}

// GetLimit returns the limit configuration for a key
func (rl *TokenBucketLimiter) GetLimit(key string) (*Limit, error) {
	return rl.getLimitForKey(key), nil
}

// getLimitForKey finds the appropriate limit for a key
func (rl *TokenBucketLimiter) getLimitForKey(key string) *Limit {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// Check for exact match or prefix match
	for prefix, limit := range rl.limits {
		if len(prefix) > 0 && len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return limit
		}
	}

	return rl.defaultLimit
}

// MultiKeyLimiter allows rate limiting across multiple dimensions
type MultiKeyLimiter struct {
	limiter *TokenBucketLimiter
	logger  *zap.Logger
}

// NewMultiKeyLimiter creates a limiter that checks multiple keys
func NewMultiKeyLimiter(redisClient *redis.Client, config *Config, logger *zap.Logger) *MultiKeyLimiter {
	return &MultiKeyLimiter{
		limiter: NewTokenBucketLimiter(redisClient, config, logger),
		logger:  logger,
	}
}

// AllowMulti checks rate limits for multiple keys (all must pass)
func (ml *MultiKeyLimiter) AllowMulti(ctx context.Context, keys []string) (bool, error) {
	for _, key := range keys {
		allowed, err := ml.limiter.Allow(ctx, key)
		if err != nil {
			return false, err
		}
		if !allowed {
			// If any key is rate limited, reject the request
			ml.logger.Warn("multi-key rate limit exceeded",
				zap.String("blocked_key", key),
				zap.Strings("all_keys", keys))
			return false, nil
		}
	}
	return true, nil
}

// SetLimit configures a rate limit
func (ml *MultiKeyLimiter) SetLimit(keyPrefix string, limit *Limit) error {
	return ml.limiter.SetLimit(keyPrefix, limit)
}

// RateLimitMiddleware creates rate limiting keys for transactions
type RateLimitMiddleware struct {
	limiter *MultiKeyLimiter
	logger  *zap.Logger
}

// NewRateLimitMiddleware creates middleware for transaction rate limiting
func NewRateLimitMiddleware(limiter *MultiKeyLimiter, logger *zap.Logger) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		limiter: limiter,
		logger:  logger,
	}
}

// CheckTransactionLimits checks rate limits for a transaction
func (rlm *RateLimitMiddleware) CheckTransactionLimits(
	ctx context.Context,
	merchantID string,
	terminalID string,
	cardBIN string,
) error {
	keys := []string{
		"merchant:" + merchantID,
		"terminal:" + terminalID,
		"card:" + cardBIN,
		"global",
	}

	allowed, err := rlm.limiter.AllowMulti(ctx, keys)
	if err != nil {
		rlm.logger.Error("rate limit check failed",
			zap.Strings("keys", keys),
			zap.Error(err))
		// Fail open on errors
		return nil
	}

	if !allowed {
		return ErrRateLimitExceeded
	}

	return nil
}

// SlidingWindowLimiter implements sliding window algorithm
type SlidingWindowLimiter struct {
	redis  *redis.Client
	logger *zap.Logger
	limit  *Limit
}

// NewSlidingWindowLimiter creates a sliding window rate limiter
func NewSlidingWindowLimiter(redisClient *redis.Client, limit *Limit, logger *zap.Logger) *SlidingWindowLimiter {
	return &SlidingWindowLimiter{
		redis:  redisClient,
		logger: logger,
		limit:  limit,
	}
}

// Allow checks if request is allowed using sliding window
func (sl *SlidingWindowLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-sl.limit.Window)

	// Lua script for sliding window
	script := `
		local key = KEYS[1]
		local window_start = tonumber(ARGV[1])
		local now = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local window = tonumber(ARGV[4])

		-- Remove old entries
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)

		-- Count current window
		local current = redis.call('ZCARD', key)

		if current < limit then
			redis.call('ZADD', key, now, now)
			redis.call('EXPIRE', key, window)
			return 1
		end

		return 0
	`

	result, err := sl.redis.Eval(ctx, script, []string{key},
		windowStart.Unix(),
		now.Unix(),
		sl.limit.Requests,
		int(sl.limit.Window.Seconds()),
	).Int()

	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// Reset resets the rate limit counter
func (sl *SlidingWindowLimiter) Reset(ctx context.Context, key string) error {
	return sl.redis.Del(ctx, key).Err()
}

// GetRemaining returns remaining requests in current window
func (sl *SlidingWindowLimiter) GetRemaining(ctx context.Context, key string) (int, error) {
	now := time.Now()
	windowStart := now.Add(-sl.limit.Window)

	// Remove expired entries and count
	pipe := sl.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", string(windowStart.Unix()))
	pipe.ZCard(ctx, key)

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	current := cmds[1].(*redis.IntCmd).Val()
	remaining := sl.limit.Requests - int(current)

	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// LeakyBucketLimiter implements leaky bucket algorithm
type LeakyBucketLimiter struct {
	redis    *redis.Client
	logger   *zap.Logger
	capacity int
	rate     time.Duration // Rate at which bucket leaks
}

// NewLeakyBucketLimiter creates a leaky bucket rate limiter
func NewLeakyBucketLimiter(redisClient *redis.Client, capacity int, rate time.Duration, logger *zap.Logger) *LeakyBucketLimiter {
	return &LeakyBucketLimiter{
		redis:    redisClient,
		logger:   logger,
		capacity: capacity,
		rate:     rate,
	}
}

// Allow checks if request can be added to bucket
func (lb *LeakyBucketLimiter) Allow(ctx context.Context, key string) (bool, error) {
	script := `
		local key = KEYS[1]
		local capacity = tonumber(ARGV[1])
		local rate = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])

		local bucket = redis.call('HMGET', key, 'level', 'last_leak')
		local level = tonumber(bucket[1]) or 0
		local last_leak = tonumber(bucket[2]) or now

		-- Leak tokens
		local elapsed = now - last_leak
		local leaked = math.floor(elapsed / rate)
		level = math.max(0, level - leaked)

		-- Try to add new request
		if level < capacity then
			level = level + 1
			redis.call('HMSET', key, 'level', level, 'last_leak', now)
			redis.call('EXPIRE', key, capacity * rate)
			return 1
		end

		return 0
	`

	result, err := lb.redis.Eval(ctx, script, []string{key},
		lb.capacity,
		lb.rate.Seconds(),
		time.Now().Unix(),
	).Int()

	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// Reset resets the bucket
func (lb *LeakyBucketLimiter) Reset(ctx context.Context, key string) error {
	return lb.redis.Del(ctx, key).Err()
}

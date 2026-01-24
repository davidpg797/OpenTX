package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

var (
	ErrCacheMiss     = errors.New("cache miss")
	ErrInvalidValue  = errors.New("invalid cache value")
	ErrCacheFull     = errors.New("cache is full")
)

// Cache interface defines cache operations
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Clear(ctx context.Context) error
}

// RedisCache implements Redis-backed cache
type RedisCache struct {
	client *redis.Client
	prefix string
	logger *zap.Logger
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(client *redis.Client, prefix string, logger *zap.Logger) *RedisCache {
	return &RedisCache{
		client: client,
		prefix: prefix,
		logger: logger,
	}
}

func (c *RedisCache) key(k string) string {
	if c.prefix == "" {
		return k
	}
	return c.prefix + ":" + k
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, c.key(key)).Bytes()
	if err == redis.Nil {
		return nil, ErrCacheMiss
	}
	if err != nil {
		c.logger.Error("cache get failed", zap.String("key", key), zap.Error(err))
		return nil, err
	}
	return val, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	err := c.client.Set(ctx, c.key(key), value, ttl).Err()
	if err != nil {
		c.logger.Error("cache set failed", zap.String("key", key), zap.Error(err))
		return err
	}
	return nil
}

func (c *RedisCache) Delete(ctx context.Context, key string) error {
	err := c.client.Del(ctx, c.key(key)).Err()
	if err != nil {
		c.logger.Error("cache delete failed", zap.String("key", key), zap.Error(err))
		return err
	}
	return nil
}

func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	result, err := c.client.Exists(ctx, c.key(key)).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

func (c *RedisCache) Clear(ctx context.Context) error {
	if c.prefix == "" {
		return c.client.FlushDB(ctx).Err()
	}

	// Delete by pattern
	iter := c.client.Scan(ctx, 0, c.prefix+":*", 100).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			c.logger.Error("failed to delete key", zap.String("key", iter.Val()), zap.Error(err))
		}
	}
	return iter.Err()
}

// MemoryCache implements in-memory cache with LRU eviction
type MemoryCache struct {
	mu       sync.RWMutex
	items    map[string]*cacheItem
	maxSize  int
	lruList  *lruList
	logger   *zap.Logger
}

type cacheItem struct {
	value      []byte
	expiration time.Time
	lruNode    *lruNode
}

type lruNode struct {
	key  string
	prev *lruNode
	next *lruNode
}

type lruList struct {
	head *lruNode
	tail *lruNode
	size int
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(maxSize int, logger *zap.Logger) *MemoryCache {
	if maxSize <= 0 {
		maxSize = 1000
	}

	mc := &MemoryCache{
		items:   make(map[string]*cacheItem),
		maxSize: maxSize,
		lruList: &lruList{},
		logger:  logger,
	}

	// Start cleanup goroutine
	go mc.cleanupLoop()

	return mc
}

func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.RLock()
	item, exists := c.items[key]
	c.mu.RUnlock()

	if !exists {
		return nil, ErrCacheMiss
	}

	// Check expiration
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		c.Delete(ctx, key)
		return nil, ErrCacheMiss
	}

	// Update LRU
	c.mu.Lock()
	c.lruList.moveToFront(item.lruNode)
	c.mu.Unlock()

	return item.value, nil
}

func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key exists
	if item, exists := c.items[key]; exists {
		item.value = value
		if ttl > 0 {
			item.expiration = time.Now().Add(ttl)
		} else {
			item.expiration = time.Time{}
		}
		c.lruList.moveToFront(item.lruNode)
		return nil
	}

	// Evict if necessary
	if len(c.items) >= c.maxSize {
		c.evictLRU()
	}

	// Add new item
	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	node := c.lruList.addToFront(key)
	c.items[key] = &cacheItem{
		value:      value,
		expiration: expiration,
		lruNode:    node,
	}

	return nil
}

func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if item, exists := c.items[key]; exists {
		c.lruList.remove(item.lruNode)
		delete(c.items, key)
	}

	return nil
}

func (c *MemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, exists := c.items[key]
	if !exists {
		return false, nil
	}

	// Check expiration
	if !item.expiration.IsZero() && time.Now().After(item.expiration) {
		return false, nil
	}

	return true, nil
}

func (c *MemoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*cacheItem)
	c.lruList = &lruList{}

	return nil
}

func (c *MemoryCache) evictLRU() {
	if c.lruList.tail == nil {
		return
	}

	key := c.lruList.tail.key
	c.lruList.remove(c.lruList.tail)
	delete(c.items, key)

	c.logger.Debug("evicted LRU item", zap.String("key", key))
}

func (c *MemoryCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

func (c *MemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var toDelete []string

	for key, item := range c.items {
		if !item.expiration.IsZero() && now.After(item.expiration) {
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		if item, exists := c.items[key]; exists {
			c.lruList.remove(item.lruNode)
			delete(c.items, key)
		}
	}

	if len(toDelete) > 0 {
		c.logger.Debug("cleaned up expired items", zap.Int("count", len(toDelete)))
	}
}

// LRU list operations
func (l *lruList) addToFront(key string) *lruNode {
	node := &lruNode{key: key}

	if l.head == nil {
		l.head = node
		l.tail = node
	} else {
		node.next = l.head
		l.head.prev = node
		l.head = node
	}

	l.size++
	return node
}

func (l *lruList) moveToFront(node *lruNode) {
	if node == l.head {
		return
	}

	l.remove(node)
	
	node.next = l.head
	node.prev = nil
	l.head.prev = node
	l.head = node

	l.size++
}

func (l *lruList) remove(node *lruNode) {
	if node.prev != nil {
		node.prev.next = node.next
	} else {
		l.head = node.next
	}

	if node.next != nil {
		node.next.prev = node.prev
	} else {
		l.tail = node.prev
	}

	l.size--
}

// TieredCache combines multiple cache layers
type TieredCache struct {
	l1     Cache // Fast local cache
	l2     Cache // Slower distributed cache
	logger *zap.Logger
}

// NewTieredCache creates a tiered cache
func NewTieredCache(l1, l2 Cache, logger *zap.Logger) *TieredCache {
	return &TieredCache{
		l1:     l1,
		l2:     l2,
		logger: logger,
	}
}

func (c *TieredCache) Get(ctx context.Context, key string) ([]byte, error) {
	// Try L1 cache first
	val, err := c.l1.Get(ctx, key)
	if err == nil {
		return val, nil
	}

	if err != ErrCacheMiss {
		c.logger.Warn("L1 cache error", zap.Error(err))
	}

	// Try L2 cache
	val, err = c.l2.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// Populate L1 cache
	go func() {
		if err := c.l1.Set(context.Background(), key, val, 5*time.Minute); err != nil {
			c.logger.Error("failed to populate L1 cache", zap.Error(err))
		}
	}()

	return val, nil
}

func (c *TieredCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Set in both caches
	err1 := c.l1.Set(ctx, key, value, ttl)
	err2 := c.l2.Set(ctx, key, value, ttl)

	if err1 != nil {
		c.logger.Error("L1 cache set failed", zap.Error(err1))
	}
	if err2 != nil {
		c.logger.Error("L2 cache set failed", zap.Error(err2))
		return err2
	}

	return nil
}

func (c *TieredCache) Delete(ctx context.Context, key string) error {
	c.l1.Delete(ctx, key)
	return c.l2.Delete(ctx, key)
}

func (c *TieredCache) Exists(ctx context.Context, key string) (bool, error) {
	exists, err := c.l1.Exists(ctx, key)
	if err == nil && exists {
		return true, nil
	}

	return c.l2.Exists(ctx, key)
}

func (c *TieredCache) Clear(ctx context.Context) error {
	c.l1.Clear(ctx)
	return c.l2.Clear(ctx)
}

// CacheWrapper provides high-level caching utilities
type CacheWrapper struct {
	cache  Cache
	logger *zap.Logger
}

// NewCacheWrapper creates a cache wrapper
func NewCacheWrapper(cache Cache, logger *zap.Logger) *CacheWrapper {
	return &CacheWrapper{
		cache:  cache,
		logger: logger,
	}
}

// GetOrSet retrieves value or sets it using the provided function
func (w *CacheWrapper) GetOrSet(ctx context.Context, key string, ttl time.Duration, fn func() (interface{}, error)) (interface{}, error) {
	// Try to get from cache
	data, err := w.cache.Get(ctx, key)
	if err == nil {
		var result interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			w.logger.Error("failed to unmarshal cached value", zap.Error(err))
		} else {
			return result, nil
		}
	}

	// Cache miss, call function
	result, err := fn()
	if err != nil {
		return nil, err
	}

	// Store in cache
	data, err = json.Marshal(result)
	if err != nil {
		w.logger.Error("failed to marshal value", zap.Error(err))
		return result, nil
	}

	if err := w.cache.Set(ctx, key, data, ttl); err != nil {
		w.logger.Error("failed to cache value", zap.Error(err))
	}

	return result, nil
}

// Invalidate removes a key from cache
func (w *CacheWrapper) Invalidate(ctx context.Context, key string) error {
	return w.cache.Delete(ctx, key)
}

// InvalidatePattern removes keys matching pattern (Redis only)
func (w *CacheWrapper) InvalidatePattern(ctx context.Context, pattern string) error {
	if rc, ok := w.cache.(*RedisCache); ok {
		iter := rc.client.Scan(ctx, 0, pattern, 100).Iterator()
		for iter.Next(ctx) {
			if err := rc.client.Del(ctx, iter.Val()).Err(); err != nil {
				w.logger.Error("failed to delete key", zap.String("key", iter.Val()), zap.Error(err))
			}
		}
		return iter.Err()
	}
	return fmt.Errorf("pattern invalidation not supported for this cache type")
}

// Remember caches the result of a function call
func Remember(ctx context.Context, cache Cache, key string, ttl time.Duration, fn func() ([]byte, error)) ([]byte, error) {
	// Try cache first
	data, err := cache.Get(ctx, key)
	if err == nil {
		return data, nil
	}

	// Call function
	data, err = fn()
	if err != nil {
		return nil, err
	}

	// Cache result
	cache.Set(ctx, key, data, ttl)

	return data, nil
}

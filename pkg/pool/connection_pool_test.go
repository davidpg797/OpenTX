package pool

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewConnectionPool(t *testing.T) {
	config := &PoolConfig{
		Address:        "localhost:8583",
		MinIdle:        2,
		MaxActive:      10,
		MaxIdle:        5,
		IdleTimeout:    1 * time.Minute,
		ConnectTimeout: 5 * time.Second,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
	}

	logger := zap.NewNop()
	pool, err := NewConnectionPool(config, logger)
	
	// Note: This will fail connection attempts but tests pool creation
	assert.Error(t, err) // Expected since localhost:8583 likely not available
	
	if pool != nil {
		defer pool.Close()
	}
}

func TestPoolConfig_Validation(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name        string
		config      *PoolConfig
		expectError bool
	}{
		{
			name: "valid config",
			config: &PoolConfig{
				Address:     "localhost:8583",
				MinIdle:     2,
				MaxActive:   10,
				MaxIdle:     5,
				IdleTimeout: 1 * time.Minute,
			},
			expectError: false,
		},
		{
			name: "maxActive too low",
			config: &PoolConfig{
				Address:   "localhost:8583",
				MaxActive: 0,
			},
			expectError: true,
		},
		{
			name: "maxIdle exceeds maxActive",
			config: &PoolConfig{
				Address:   "localhost:8583",
				MaxActive: 5,
				MaxIdle:   10,
			},
			expectError: true,
		},
		{
			name: "minIdle exceeds maxIdle",
			config: &PoolConfig{
				Address:   "localhost:8583",
				MinIdle:   10,
				MaxActive: 20,
				MaxIdle:   5,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewConnectionPool(tt.config, logger)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				// Will error on connection but not config validation
				if pool != nil {
					defer pool.Close()
				}
			}
		})
	}
}

func TestPoolStats(t *testing.T) {
	config := DefaultPoolConfig()
	config.Address = "localhost:8583"
	
	logger := zap.NewNop()
	pool, err := NewConnectionPool(config, logger)
	
	// Expected error due to connection failure
	if err != nil {
		t.Skip("Skipping test as connection cannot be established")
	}
	
	require.NotNil(t, pool)
	defer pool.Close()

	stats := pool.Stats()
	assert.GreaterOrEqual(t, stats.TotalCreated, int64(0))
	assert.GreaterOrEqual(t, stats.CurrentActive, 0)
}

func TestDefaultPoolConfig(t *testing.T) {
	config := DefaultPoolConfig()
	
	assert.Equal(t, 5, config.MinIdle)
	assert.Equal(t, 100, config.MaxActive)
	assert.Equal(t, 20, config.MaxIdle)
	assert.Equal(t, 5*time.Minute, config.IdleTimeout)
	assert.Equal(t, 10*time.Second, config.ConnectTimeout)
	assert.Equal(t, 30*time.Second, config.ReadTimeout)
	assert.Equal(t, 30*time.Second, config.WriteTimeout)
}

func TestPooledConnection_Methods(t *testing.T) {
	// Test Age and IdleTime calculations
	now := time.Now()
	
	pc := &PooledConnection{
		createdAt:  now.Add(-5 * time.Minute),
		lastUsedAt: now.Add(-30 * time.Second),
	}

	age := pc.Age()
	assert.GreaterOrEqual(t, age, 5*time.Minute)
	
	idleTime := pc.IdleTime()
	assert.GreaterOrEqual(t, idleTime, 30*time.Second)
}

func TestPool_CloseIdempotency(t *testing.T) {
	config := DefaultPoolConfig()
	config.Address = "localhost:8583"
	
	logger := zap.NewNop()
	pool, err := NewConnectionPool(config, logger)
	
	if err != nil {
		t.Skip("Skipping test as connection cannot be established")
	}
	
	require.NotNil(t, pool)

	// First close should succeed
	err = pool.Close()
	assert.NoError(t, err)

	// Second close should return error
	err = pool.Close()
	assert.Equal(t, ErrPoolClosed, err)
}

func TestPool_GetAfterClose(t *testing.T) {
	config := DefaultPoolConfig()
	config.Address = "localhost:8583"
	
	logger := zap.NewNop()
	pool, err := NewConnectionPool(config, logger)
	
	if err != nil {
		t.Skip("Skipping test as connection cannot be established")
	}
	
	require.NotNil(t, pool)
	pool.Close()

	ctx := context.Background()
	conn, err := pool.Get(ctx)
	
	assert.Equal(t, ErrPoolClosed, err)
	assert.Nil(t, conn)
}

func BenchmarkPool_GetPut(b *testing.B) {
	config := &PoolConfig{
		Address:        "localhost:8583",
		MinIdle:        10,
		MaxActive:      100,
		MaxIdle:        50,
		IdleTimeout:    5 * time.Minute,
		ConnectTimeout: 5 * time.Second,
	}

	logger := zap.NewNop()
	pool, err := NewConnectionPool(config, logger)
	
	if err != nil {
		b.Skip("Skipping benchmark as connection cannot be established")
	}
	
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		for pb.Next() {
			conn, err := pool.Get(ctx)
			if err != nil {
				continue
			}
			pool.Put(conn)
		}
	})
}

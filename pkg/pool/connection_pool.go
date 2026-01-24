package pool

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrPoolClosed     = errors.New("connection pool is closed")
	ErrPoolExhausted  = errors.New("connection pool exhausted")
	ErrInvalidConn    = errors.New("invalid connection")
	ErrConnTimeout    = errors.New("connection timeout")
)

// ConnectionPool manages a pool of ISO 8583 network connections
type ConnectionPool struct {
	addr           string
	minIdle        int
	maxActive      int
	maxIdle        int
	idleTimeout    time.Duration
	connectTimeout time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration

	mu          sync.RWMutex
	connections chan *PooledConnection
	active      int
	closed      bool

	logger *zap.Logger

	// Metrics
	stats PoolStats
}

// PooledConnection wraps a net.Conn with pool metadata
type PooledConnection struct {
	conn       net.Conn
	createdAt  time.Time
	lastUsedAt time.Time
	usageCount int
	pool       *ConnectionPool
}

// PoolStats tracks connection pool statistics
type PoolStats struct {
	mu              sync.RWMutex
	TotalCreated    int64
	TotalClosed     int64
	TotalRequests   int64
	TotalWaits      int64
	AverageWaitTime time.Duration
	CurrentActive   int
	CurrentIdle     int
}

// PoolConfig holds connection pool configuration
type PoolConfig struct {
	Address        string
	MinIdle        int
	MaxActive      int
	MaxIdle        int
	IdleTimeout    time.Duration
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
}

// DefaultPoolConfig returns default configuration
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MinIdle:        5,
		MaxActive:      100,
		MaxIdle:        20,
		IdleTimeout:    5 * time.Minute,
		ConnectTimeout: 10 * time.Second,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
	}
}

// NewConnectionPool creates a new connection pool
func NewConnectionPool(config *PoolConfig, logger *zap.Logger) (*ConnectionPool, error) {
	if config.MaxActive < 1 {
		return nil, errors.New("maxActive must be at least 1")
	}
	if config.MaxIdle > config.MaxActive {
		return nil, errors.New("maxIdle cannot exceed maxActive")
	}
	if config.MinIdle > config.MaxIdle {
		return nil, errors.New("minIdle cannot exceed maxIdle")
	}

	pool := &ConnectionPool{
		addr:           config.Address,
		minIdle:        config.MinIdle,
		maxActive:      config.MaxActive,
		maxIdle:        config.MaxIdle,
		idleTimeout:    config.IdleTimeout,
		connectTimeout: config.ConnectTimeout,
		readTimeout:    config.ReadTimeout,
		writeTimeout:   config.WriteTimeout,
		connections:    make(chan *PooledConnection, config.MaxIdle),
		logger:         logger,
	}

	// Pre-populate with minimum idle connections
	for i := 0; i < config.MinIdle; i++ {
		conn, err := pool.createConnection()
		if err != nil {
			pool.logger.Error("failed to create initial connection",
				zap.Error(err),
				zap.Int("connection_number", i))
			continue
		}
		pool.connections <- conn
	}

	// Start background goroutine to maintain minimum idle connections
	go pool.maintainIdleConnections()

	return pool, nil
}

// Get retrieves a connection from the pool
func (p *ConnectionPool) Get(ctx context.Context) (*PooledConnection, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrPoolClosed
	}
	p.mu.RUnlock()

	p.stats.mu.Lock()
	p.stats.TotalRequests++
	p.stats.mu.Unlock()

	startWait := time.Now()

	select {
	case conn := <-p.connections:
		// Got idle connection
		if p.isConnectionValid(conn) {
			conn.lastUsedAt = time.Now()
			conn.usageCount++
			return conn, nil
		}
		// Invalid connection, close and create new
		conn.close()
		return p.createAndActivate()

	case <-ctx.Done():
		return nil, ctx.Err()

	default:
		// No idle connections, try to create new
		p.mu.Lock()
		if p.active < p.maxActive {
			p.mu.Unlock()
			
			waitTime := time.Since(startWait)
			p.stats.mu.Lock()
			p.stats.TotalWaits++
			p.stats.AverageWaitTime = (p.stats.AverageWaitTime + waitTime) / 2
			p.stats.mu.Unlock()
			
			return p.createAndActivate()
		}
		p.mu.Unlock()

		// Pool exhausted, wait for available connection
		select {
		case conn := <-p.connections:
			if p.isConnectionValid(conn) {
				conn.lastUsedAt = time.Now()
				conn.usageCount++
				return conn, nil
			}
			conn.close()
			return p.createAndActivate()

		case <-ctx.Done():
			return nil, ctx.Err()

		case <-time.After(p.connectTimeout):
			return nil, ErrConnTimeout
		}
	}
}

// Put returns a connection to the pool
func (p *ConnectionPool) Put(conn *PooledConnection) error {
	if conn == nil {
		return ErrInvalidConn
	}

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		conn.close()
		return ErrPoolClosed
	}
	p.mu.RUnlock()

	// Check if connection is still valid
	if !p.isConnectionValid(conn) {
		conn.close()
		return nil
	}

	// Try to return to pool
	select {
	case p.connections <- conn:
		// Successfully returned to pool
		return nil
	default:
		// Pool is full, close the connection
		conn.close()
		return nil
	}
}

// Close shuts down the connection pool
func (p *ConnectionPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrPoolClosed
	}
	p.closed = true
	p.mu.Unlock()

	close(p.connections)

	// Close all idle connections
	for conn := range p.connections {
		conn.close()
	}

	p.logger.Info("connection pool closed",
		zap.String("address", p.addr))

	return nil
}

// Stats returns current pool statistics
func (p *ConnectionPool) Stats() PoolStats {
	p.stats.mu.RLock()
	defer p.stats.mu.RUnlock()

	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := p.stats
	stats.CurrentActive = p.active
	stats.CurrentIdle = len(p.connections)

	return stats
}

// createConnection creates a new network connection
func (p *ConnectionPool) createConnection() (*PooledConnection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.connectTimeout)
	defer cancel()

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return nil, err
	}

	pooledConn := &PooledConnection{
		conn:       conn,
		createdAt:  time.Now(),
		lastUsedAt: time.Now(),
		usageCount: 0,
		pool:       p,
	}

	p.stats.mu.Lock()
	p.stats.TotalCreated++
	p.stats.mu.Unlock()

	p.logger.Debug("created new connection",
		zap.String("address", p.addr))

	return pooledConn, nil
}

// createAndActivate creates a connection and increments active count
func (p *ConnectionPool) createAndActivate() (*PooledConnection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active >= p.maxActive {
		return nil, ErrPoolExhausted
	}

	conn, err := p.createConnection()
	if err != nil {
		return nil, err
	}

	p.active++
	p.stats.mu.Lock()
	p.stats.CurrentActive = p.active
	p.stats.mu.Unlock()

	return conn, nil
}

// isConnectionValid checks if connection is still valid
func (p *ConnectionPool) isConnectionValid(conn *PooledConnection) bool {
	if conn == nil || conn.conn == nil {
		return false
	}

	// Check idle timeout
	if time.Since(conn.lastUsedAt) > p.idleTimeout {
		p.logger.Debug("connection expired due to idle timeout",
			zap.Duration("idle_time", time.Since(conn.lastUsedAt)))
		return false
	}

	// Check if connection is alive by setting deadline and reading
	conn.conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	one := make([]byte, 1)
	if _, err := conn.conn.Read(one); err == nil {
		// Data available, connection might be in bad state
		return false
	}
	conn.conn.SetReadDeadline(time.Time{}) // Clear deadline

	return true
}

// maintainIdleConnections runs in background to maintain minimum idle connections
func (p *ConnectionPool) maintainIdleConnections() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.RLock()
		if p.closed {
			p.mu.RUnlock()
			return
		}
		p.mu.RUnlock()

		currentIdle := len(p.connections)
		if currentIdle < p.minIdle {
			needed := p.minIdle - currentIdle

			p.mu.Lock()
			canCreate := p.maxActive - p.active
			if canCreate > needed {
				canCreate = needed
			}
			p.mu.Unlock()

			for i := 0; i < canCreate; i++ {
				conn, err := p.createConnection()
				if err != nil {
					p.logger.Error("failed to create idle connection",
						zap.Error(err))
					continue
				}

				select {
				case p.connections <- conn:
					p.mu.Lock()
					p.active++
					p.mu.Unlock()
				default:
					conn.close()
				}
			}
		}
	}
}

// PooledConnection methods

// Read implements io.Reader
func (pc *PooledConnection) Read(b []byte) (int, error) {
	if pc.pool.readTimeout > 0 {
		pc.conn.SetReadDeadline(time.Now().Add(pc.pool.readTimeout))
	}
	return pc.conn.Read(b)
}

// Write implements io.Writer
func (pc *PooledConnection) Write(b []byte) (int, error) {
	if pc.pool.writeTimeout > 0 {
		pc.conn.SetWriteDeadline(time.Now().Add(pc.pool.writeTimeout))
	}
	return pc.conn.Write(b)
}

// Close returns the connection to the pool
func (pc *PooledConnection) Close() error {
	return pc.pool.Put(pc)
}

// close actually closes the underlying connection
func (pc *PooledConnection) close() error {
	pc.pool.mu.Lock()
	if pc.pool.active > 0 {
		pc.pool.active--
	}
	pc.pool.mu.Unlock()

	pc.pool.stats.mu.Lock()
	pc.pool.stats.TotalClosed++
	pc.pool.stats.CurrentActive = pc.pool.active
	pc.pool.stats.mu.Unlock()

	if pc.conn != nil {
		return pc.conn.Close()
	}
	return nil
}

// Discard permanently closes the connection without returning to pool
func (pc *PooledConnection) Discard() error {
	return pc.close()
}

// Age returns how long the connection has existed
func (pc *PooledConnection) Age() time.Duration {
	return time.Since(pc.createdAt)
}

// IdleTime returns how long the connection has been idle
func (pc *PooledConnection) IdleTime() time.Duration {
	return time.Since(pc.lastUsedAt)
}

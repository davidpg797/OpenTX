package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var (
	ErrConfigNotFound   = errors.New("configuration not found")
	ErrInvalidConfig    = errors.New("invalid configuration")
	ErrValidationFailed = errors.New("configuration validation failed")
)

// Config represents application configuration
type Config struct {
	Server    ServerConfig    `yaml:"server" json:"server"`
	Database  DatabaseConfig  `yaml:"database" json:"database"`
	Redis     RedisConfig     `yaml:"redis" json:"redis"`
	Kafka     KafkaConfig     `yaml:"kafka" json:"kafka"`
	Security  SecurityConfig  `yaml:"security" json:"security"`
	Networks  []NetworkConfig `yaml:"networks" json:"networks"`
	Observability ObservabilityConfig `yaml:"observability" json:"observability"`
	Features  FeatureFlags    `yaml:"features" json:"features"`
}

// ServerConfig holds server settings
type ServerConfig struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	ReadTimeout     time.Duration `yaml:"read_timeout" json:"read_timeout"`
	WriteTimeout    time.Duration `yaml:"write_timeout" json:"write_timeout"`
	ShutdownTimeout time.Duration `yaml:"shutdown_timeout" json:"shutdown_timeout"`
	MaxHeaderBytes  int           `yaml:"max_header_bytes" json:"max_header_bytes"`
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	Database        string        `yaml:"database" json:"database"`
	User            string        `yaml:"user" json:"user"`
	Password        string        `yaml:"password" json:"password"`
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	SSLMode         string        `yaml:"ssl_mode" json:"ssl_mode"`
}

// RedisConfig holds Redis settings
type RedisConfig struct {
	Host         string        `yaml:"host" json:"host"`
	Port         int           `yaml:"port" json:"port"`
	Password     string        `yaml:"password" json:"password"`
	DB           int           `yaml:"db" json:"db"`
	PoolSize     int           `yaml:"pool_size" json:"pool_size"`
	MinIdleConns int           `yaml:"min_idle_conns" json:"min_idle_conns"`
	MaxRetries   int           `yaml:"max_retries" json:"max_retries"`
	DialTimeout  time.Duration `yaml:"dial_timeout" json:"dial_timeout"`
}

// KafkaConfig holds Kafka settings
type KafkaConfig struct {
	Brokers       []string      `yaml:"brokers" json:"brokers"`
	Topic         string        `yaml:"topic" json:"topic"`
	ConsumerGroup string        `yaml:"consumer_group" json:"consumer_group"`
	Partitions    int           `yaml:"partitions" json:"partitions"`
	Replication   int           `yaml:"replication" json:"replication"`
	BatchSize     int           `yaml:"batch_size" json:"batch_size"`
	Timeout       time.Duration `yaml:"timeout" json:"timeout"`
}

// SecurityConfig holds security settings
type SecurityConfig struct {
	EncryptionKey     string `yaml:"encryption_key" json:"encryption_key"`
	SigningKey        string `yaml:"signing_key" json:"signing_key"`
	HSMEnabled        bool   `yaml:"hsm_enabled" json:"hsm_enabled"`
	HSMLibrary        string `yaml:"hsm_library" json:"hsm_library"`
	HSMSlot           int    `yaml:"hsm_slot" json:"hsm_slot"`
	HSMPin            string `yaml:"hsm_pin" json:"hsm_pin"`
	TLSEnabled        bool   `yaml:"tls_enabled" json:"tls_enabled"`
	TLSCertFile       string `yaml:"tls_cert_file" json:"tls_cert_file"`
	TLSKeyFile        string `yaml:"tls_key_file" json:"tls_key_file"`
	MTLSEnabled       bool   `yaml:"mtls_enabled" json:"mtls_enabled"`
	MTLSCAFile        string `yaml:"mtls_ca_file" json:"mtls_ca_file"`
}

// NetworkConfig holds ISO 8583 network settings
type NetworkConfig struct {
	Name              string        `yaml:"name" json:"name"`
	Type              string        `yaml:"type" json:"type"` // visa, mastercard, etc.
	Host              string        `yaml:"host" json:"host"`
	Port              int           `yaml:"port" json:"port"`
	Timeout           time.Duration `yaml:"timeout" json:"timeout"`
	MaxConnections    int           `yaml:"max_connections" json:"max_connections"`
	IdleTimeout       time.Duration `yaml:"idle_timeout" json:"idle_timeout"`
	HealthCheckPeriod time.Duration `yaml:"health_check_period" json:"health_check_period"`
	RetryAttempts     int           `yaml:"retry_attempts" json:"retry_attempts"`
	RetryDelay        time.Duration `yaml:"retry_delay" json:"retry_delay"`
	TLSEnabled        bool          `yaml:"tls_enabled" json:"tls_enabled"`
	StandinEnabled    bool          `yaml:"standin_enabled" json:"standin_enabled"`
}

// ObservabilityConfig holds observability settings
type ObservabilityConfig struct {
	LogLevel          string `yaml:"log_level" json:"log_level"`
	LogFormat         string `yaml:"log_format" json:"log_format"`
	TracingEnabled    bool   `yaml:"tracing_enabled" json:"tracing_enabled"`
	TracingSampleRate float64 `yaml:"tracing_sample_rate" json:"tracing_sample_rate"`
	JaegerEndpoint    string `yaml:"jaeger_endpoint" json:"jaeger_endpoint"`
	MetricsEnabled    bool   `yaml:"metrics_enabled" json:"metrics_enabled"`
	MetricsPort       int    `yaml:"metrics_port" json:"metrics_port"`
}

// FeatureFlags holds feature toggles
type FeatureFlags struct {
	IdempotencyEnabled  bool `yaml:"idempotency_enabled" json:"idempotency_enabled"`
	FraudDetection      bool `yaml:"fraud_detection" json:"fraud_detection"`
	RateLimiting        bool `yaml:"rate_limiting" json:"rate_limiting"`
	CircuitBreaker      bool `yaml:"circuit_breaker" json:"circuit_breaker"`
	WebhooksEnabled     bool `yaml:"webhooks_enabled" json:"webhooks_enabled"`
	BatchProcessing     bool `yaml:"batch_processing" json:"batch_processing"`
	ReconciliationAuto  bool `yaml:"reconciliation_auto" json:"reconciliation_auto"`
}

// Manager manages application configuration
type Manager struct {
	mu          sync.RWMutex
	config      *Config
	filePath    string
	logger      *zap.Logger
	validators  []Validator
	watchers    []ConfigWatcher
}

// Validator validates configuration
type Validator interface {
	Validate(config *Config) error
}

// ConfigWatcher watches for config changes
type ConfigWatcher interface {
	OnConfigChange(old, new *Config)
}

// NewManager creates a new config manager
func NewManager(filePath string, logger *zap.Logger) *Manager {
	return &Manager{
		filePath:   filePath,
		logger:     logger,
		validators: make([]Validator, 0),
		watchers:   make([]ConfigWatcher, 0),
	}
}

// Load loads configuration from file
func (m *Manager) Load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply environment variable overrides
	m.applyEnvOverrides(&config)

	// Validate configuration
	if err := m.validate(&config); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	m.mu.Lock()
	oldConfig := m.config
	m.config = &config
	watchers := make([]ConfigWatcher, len(m.watchers))
	copy(watchers, m.watchers)
	m.mu.Unlock()

	m.logger.Info("configuration loaded", zap.String("file", m.filePath))

	// Notify watchers
	if oldConfig != nil {
		for _, watcher := range watchers {
			go watcher.OnConfigChange(oldConfig, &config)
		}
	}

	return nil
}

// Get returns current configuration
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// Update updates configuration
func (m *Manager) Update(config *Config) error {
	if err := m.validate(config); err != nil {
		return err
	}

	m.mu.Lock()
	oldConfig := m.config
	m.config = config
	watchers := make([]ConfigWatcher, len(m.watchers))
	copy(watchers, m.watchers)
	m.mu.Unlock()

	m.logger.Info("configuration updated")

	// Notify watchers
	for _, watcher := range watchers {
		go watcher.OnConfigChange(oldConfig, config)
	}

	return nil
}

// Save saves configuration to file
func (m *Manager) Save() error {
	m.mu.RLock()
	config := m.config
	m.mu.RUnlock()

	if config == nil {
		return ErrConfigNotFound
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(m.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	m.logger.Info("configuration saved", zap.String("file", m.filePath))
	return nil
}

// RegisterValidator registers a validator
func (m *Manager) RegisterValidator(validator Validator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validators = append(m.validators, validator)
}

// RegisterWatcher registers a config watcher
func (m *Manager) RegisterWatcher(watcher ConfigWatcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watchers = append(m.watchers, watcher)
}

func (m *Manager) validate(config *Config) error {
	// Basic validation
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("%w: invalid server port", ErrValidationFailed)
	}

	if config.Database.Host == "" {
		return fmt.Errorf("%w: database host is required", ErrValidationFailed)
	}

	if config.Redis.Host == "" {
		return fmt.Errorf("%w: redis host is required", ErrValidationFailed)
	}

	// Run custom validators
	for _, validator := range m.validators {
		if err := validator.Validate(config); err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) applyEnvOverrides(config *Config) {
	// Server
	if val := os.Getenv("SERVER_PORT"); val != "" {
		var port int
		fmt.Sscanf(val, "%d", &port)
		if port > 0 {
			config.Server.Port = port
		}
	}

	// Database
	if val := os.Getenv("DB_HOST"); val != "" {
		config.Database.Host = val
	}
	if val := os.Getenv("DB_PORT"); val != "" {
		var port int
		fmt.Sscanf(val, "%d", &port)
		if port > 0 {
			config.Database.Port = port
		}
	}
	if val := os.Getenv("DB_USER"); val != "" {
		config.Database.User = val
	}
	if val := os.Getenv("DB_PASSWORD"); val != "" {
		config.Database.Password = val
	}
	if val := os.Getenv("DB_NAME"); val != "" {
		config.Database.Database = val
	}

	// Redis
	if val := os.Getenv("REDIS_HOST"); val != "" {
		config.Redis.Host = val
	}
	if val := os.Getenv("REDIS_PORT"); val != "" {
		var port int
		fmt.Sscanf(val, "%d", &port)
		if port > 0 {
			config.Redis.Port = port
		}
	}
	if val := os.Getenv("REDIS_PASSWORD"); val != "" {
		config.Redis.Password = val
	}

	// Kafka
	if val := os.Getenv("KAFKA_BROKERS"); val != "" {
		// Simple comma-separated parsing
		config.Kafka.Brokers = []string{val}
	}

	// Security
	if val := os.Getenv("ENCRYPTION_KEY"); val != "" {
		config.Security.EncryptionKey = val
	}
	if val := os.Getenv("SIGNING_KEY"); val != "" {
		config.Security.SigningKey = val
	}
}

// GetJSON returns configuration as JSON
func (m *Manager) GetJSON() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return "", ErrConfigNotFound
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// GetYAML returns configuration as YAML
func (m *Manager) GetYAML() (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.config == nil {
		return "", ErrConfigNotFound
	}

	data, err := yaml.Marshal(m.config)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     10 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 30 * time.Second,
			MaxHeaderBytes:  1 << 20, // 1 MB
		},
		Database: DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Database:        "opentx",
			User:            "opentx",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
			SSLMode:         "disable",
		},
		Redis: RedisConfig{
			Host:         "localhost",
			Port:         6379,
			DB:           0,
			PoolSize:     10,
			MinIdleConns: 2,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
		},
		Kafka: KafkaConfig{
			Brokers:       []string{"localhost:9092"},
			Topic:         "transactions",
			ConsumerGroup: "opentx-gateway",
			Partitions:    3,
			Replication:   1,
			BatchSize:     100,
			Timeout:       10 * time.Second,
		},
		Security: SecurityConfig{
			HSMEnabled:  false,
			TLSEnabled:  false,
			MTLSEnabled: false,
		},
		Observability: ObservabilityConfig{
			LogLevel:          "info",
			LogFormat:         "json",
			TracingEnabled:    true,
			TracingSampleRate: 0.1,
			JaegerEndpoint:    "http://localhost:14268/api/traces",
			MetricsEnabled:    true,
			MetricsPort:       9090,
		},
		Features: FeatureFlags{
			IdempotencyEnabled: true,
			FraudDetection:     true,
			RateLimiting:       true,
			CircuitBreaker:     true,
			WebhooksEnabled:    true,
			BatchProcessing:    true,
			ReconciliationAuto: false,
		},
	}
}

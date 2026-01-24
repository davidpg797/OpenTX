package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

var (
	ErrWebhookFailed      = errors.New("webhook delivery failed")
	ErrInvalidSignature   = errors.New("invalid webhook signature")
	ErrWebhookTimeout     = errors.New("webhook delivery timeout")
)

// Event represents a webhook event
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
	Version   string                 `json:"version"`
}

// Subscription represents a webhook subscription
type Subscription struct {
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Events      []string `json:"events"`
	Secret      string   `json:"secret"`
	Active      bool     `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
	RetryPolicy RetryPolicy `json:"retry_policy"`
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	InitialDelay time.Duration `json:"initial_delay"`
	MaxDelay    time.Duration `json:"max_delay"`
	Multiplier  float64       `json:"multiplier"`
}

// DeliveryResult represents webhook delivery outcome
type DeliveryResult struct {
	EventID      string        `json:"event_id"`
	SubscriptionID string      `json:"subscription_id"`
	Attempt      int           `json:"attempt"`
	StatusCode   int           `json:"status_code"`
	Duration     time.Duration `json:"duration"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
}

// Webhook manages webhook delivery
type Webhook struct {
	subscriptions sync.Map // map[string]*Subscription
	deliveryQueue chan *deliveryTask
	workers       int
	timeout       time.Duration
	client        *http.Client
	logger        *zap.Logger
	
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	
	// Storage callbacks
	onDelivery    func(*DeliveryResult)
}

type deliveryTask struct {
	event        *Event
	subscription *Subscription
	attempt      int
}

// Config configures the webhook manager
type Config struct {
	Workers      int
	QueueSize    int
	Timeout      time.Duration
	OnDelivery   func(*DeliveryResult)
}

// NewWebhook creates a new webhook manager
func NewWebhook(config Config, logger *zap.Logger) *Webhook {
	if config.Workers == 0 {
		config.Workers = 10
	}
	if config.QueueSize == 0 {
		config.QueueSize = 1000
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	wh := &Webhook{
		deliveryQueue: make(chan *deliveryTask, config.QueueSize),
		workers:       config.Workers,
		timeout:       config.Timeout,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		onDelivery:    config.OnDelivery,
		client: &http.Client{
			Timeout: config.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}

	return wh
}

// Start starts webhook workers
func (wh *Webhook) Start() {
	for i := 0; i < wh.workers; i++ {
		wh.wg.Add(1)
		go wh.worker(i)
	}
	wh.logger.Info("webhook workers started", zap.Int("workers", wh.workers))
}

// Stop stops webhook workers
func (wh *Webhook) Stop() {
	wh.cancel()
	close(wh.deliveryQueue)
	wh.wg.Wait()
	wh.logger.Info("webhook workers stopped")
}

// Subscribe adds a webhook subscription
func (wh *Webhook) Subscribe(sub *Subscription) error {
	if sub.ID == "" {
		return errors.New("subscription ID is required")
	}
	if sub.URL == "" {
		return errors.New("subscription URL is required")
	}
	if len(sub.Events) == 0 {
		return errors.New("at least one event type is required")
	}

	// Set defaults
	if sub.RetryPolicy.MaxAttempts == 0 {
		sub.RetryPolicy.MaxAttempts = 5
	}
	if sub.RetryPolicy.InitialDelay == 0 {
		sub.RetryPolicy.InitialDelay = 1 * time.Second
	}
	if sub.RetryPolicy.MaxDelay == 0 {
		sub.RetryPolicy.MaxDelay = 30 * time.Second
	}
	if sub.RetryPolicy.Multiplier == 0 {
		sub.RetryPolicy.Multiplier = 2.0
	}

	sub.Active = true
	sub.CreatedAt = time.Now()

	wh.subscriptions.Store(sub.ID, sub)
	wh.logger.Info("webhook subscription added",
		zap.String("id", sub.ID),
		zap.String("url", sub.URL),
		zap.Strings("events", sub.Events))

	return nil
}

// Unsubscribe removes a webhook subscription
func (wh *Webhook) Unsubscribe(id string) {
	wh.subscriptions.Delete(id)
	wh.logger.Info("webhook subscription removed", zap.String("id", id))
}

// Publish publishes an event to all matching subscriptions
func (wh *Webhook) Publish(event *Event) error {
	if event.ID == "" {
		return errors.New("event ID is required")
	}
	if event.Type == "" {
		return errors.New("event type is required")
	}

	event.Timestamp = time.Now()

	var count int
	wh.subscriptions.Range(func(key, value interface{}) bool {
		sub := value.(*Subscription)
		
		if !sub.Active {
			return true
		}

		// Check if subscription is interested in this event
		if !wh.matchesEvent(sub, event.Type) {
			return true
		}

		// Queue delivery
		select {
		case wh.deliveryQueue <- &deliveryTask{
			event:        event,
			subscription: sub,
			attempt:      0,
		}:
			count++
		default:
			wh.logger.Warn("webhook queue full, dropping event",
				zap.String("event_id", event.ID),
				zap.String("subscription_id", sub.ID))
		}

		return true
	})

	wh.logger.Debug("event published",
		zap.String("event_id", event.ID),
		zap.String("type", event.Type),
		zap.Int("subscribers", count))

	return nil
}

func (wh *Webhook) matchesEvent(sub *Subscription, eventType string) bool {
	for _, et := range sub.Events {
		if et == "*" || et == eventType {
			return true
		}
	}
	return false
}

func (wh *Webhook) worker(id int) {
	defer wh.wg.Done()

	wh.logger.Info("webhook worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-wh.ctx.Done():
			return
		case task, ok := <-wh.deliveryQueue:
			if !ok {
				return
			}
			wh.deliver(task)
		}
	}
}

func (wh *Webhook) deliver(task *deliveryTask) {
	result := &DeliveryResult{
		EventID:        task.event.ID,
		SubscriptionID: task.subscription.ID,
		Attempt:        task.attempt + 1,
		Timestamp:      time.Now(),
	}

	start := time.Now()

	// Prepare payload
	payload, err := json.Marshal(task.event)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to marshal event: %v", err)
		wh.recordDelivery(result)
		return
	}

	// Create request
	req, err := http.NewRequestWithContext(wh.ctx, "POST", task.subscription.URL, bytes.NewReader(payload))
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("failed to create request: %v", err)
		wh.recordDelivery(result)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event-ID", task.event.ID)
	req.Header.Set("X-Webhook-Event-Type", task.event.Type)
	req.Header.Set("X-Webhook-Delivery-ID", fmt.Sprintf("%d", task.attempt+1))

	// Add signature
	if task.subscription.Secret != "" {
		signature := wh.generateSignature(payload, task.subscription.Secret)
		req.Header.Set("X-Webhook-Signature", signature)
	}

	// Send request
	resp, err := wh.client.Do(req)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("request failed: %v", err)
		result.Duration = time.Since(start)
		wh.recordDelivery(result)
		wh.scheduleRetry(task)
		return
	}
	defer resp.Body.Close()

	// Read response (limit to 1MB)
	_, _ = io.CopyN(io.Discard, resp.Body, 1024*1024)

	result.StatusCode = resp.StatusCode
	result.Duration = time.Since(start)

	// Check if successful
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Success = true
		wh.logger.Debug("webhook delivered successfully",
			zap.String("event_id", task.event.ID),
			zap.String("subscription_id", task.subscription.ID),
			zap.Int("status_code", resp.StatusCode),
			zap.Duration("duration", result.Duration))
	} else {
		result.Success = false
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		wh.logger.Warn("webhook delivery failed",
			zap.String("event_id", task.event.ID),
			zap.String("subscription_id", task.subscription.ID),
			zap.Int("status_code", resp.StatusCode))
		wh.scheduleRetry(task)
	}

	wh.recordDelivery(result)
}

func (wh *Webhook) scheduleRetry(task *deliveryTask) {
	if task.attempt >= task.subscription.RetryPolicy.MaxAttempts-1 {
		wh.logger.Error("max retry attempts reached",
			zap.String("event_id", task.event.ID),
			zap.String("subscription_id", task.subscription.ID),
			zap.Int("attempts", task.attempt+1))
		return
	}

	// Calculate delay with exponential backoff
	delay := wh.calculateRetryDelay(task.attempt, task.subscription.RetryPolicy)

	wh.logger.Info("scheduling retry",
		zap.String("event_id", task.event.ID),
		zap.String("subscription_id", task.subscription.ID),
		zap.Int("attempt", task.attempt+2),
		zap.Duration("delay", delay))

	// Schedule retry
	time.AfterFunc(delay, func() {
		task.attempt++
		select {
		case wh.deliveryQueue <- task:
		default:
			wh.logger.Warn("failed to queue retry, queue full",
				zap.String("event_id", task.event.ID))
		}
	})
}

func (wh *Webhook) calculateRetryDelay(attempt int, policy RetryPolicy) time.Duration {
	delay := float64(policy.InitialDelay)
	for i := 0; i < attempt; i++ {
		delay *= policy.Multiplier
	}

	if time.Duration(delay) > policy.MaxDelay {
		return policy.MaxDelay
	}

	return time.Duration(delay)
}

func (wh *Webhook) generateSignature(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature verifies webhook signature
func VerifySignature(payload []byte, signature, secret string) bool {
	expected := "sha256=" + hex.EncodeToString(hmac.New(sha256.New, []byte(secret)).Sum(payload))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func (wh *Webhook) recordDelivery(result *DeliveryResult) {
	if wh.onDelivery != nil {
		wh.onDelivery(result)
	}
}

// GetSubscriptions returns all subscriptions
func (wh *Webhook) GetSubscriptions() []*Subscription {
	var subs []*Subscription
	wh.subscriptions.Range(func(key, value interface{}) bool {
		subs = append(subs, value.(*Subscription))
		return true
	})
	return subs
}

// GetSubscription returns a subscription by ID
func (wh *Webhook) GetSubscription(id string) (*Subscription, bool) {
	val, ok := wh.subscriptions.Load(id)
	if !ok {
		return nil, false
	}
	return val.(*Subscription), true
}

package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// Schema represents GraphQL schema
type Schema struct {
	queries   map[string]*QueryResolver
	mutations map[string]*MutationResolver
	logger    *zap.Logger
}

// QueryResolver resolves a query
type QueryResolver struct {
	Name        string
	Description string
	Args        []Argument
	Resolve     func(context.Context, map[string]interface{}) (interface{}, error)
}

// MutationResolver resolves a mutation
type MutationResolver struct {
	Name        string
	Description string
	Args        []Argument
	Resolve     func(context.Context, map[string]interface{}) (interface{}, error)
}

// Argument represents a GraphQL argument
type Argument struct {
	Name        string
	Type        string
	Required    bool
	Description string
}

// Request represents a GraphQL request
type Request struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName,omitempty"`
	Variables     map[string]interface{} `json:"variables,omitempty"`
}

// Response represents a GraphQL response
type Response struct {
	Data   interface{}   `json:"data,omitempty"`
	Errors []ErrorObject `json:"errors,omitempty"`
}

// ErrorObject represents a GraphQL error
type ErrorObject struct {
	Message    string                 `json:"message"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// NewSchema creates a new GraphQL schema
func NewSchema(logger *zap.Logger) *Schema {
	return &Schema{
		queries:   make(map[string]*QueryResolver),
		mutations: make(map[string]*MutationResolver),
		logger:    logger,
	}
}

// RegisterQuery registers a query resolver
func (s *Schema) RegisterQuery(resolver *QueryResolver) {
	s.queries[resolver.Name] = resolver
	s.logger.Info("query registered", zap.String("name", resolver.Name))
}

// RegisterMutation registers a mutation resolver
func (s *Schema) RegisterMutation(resolver *MutationResolver) {
	s.mutations[resolver.Name] = resolver
	s.logger.Info("mutation registered", zap.String("name", resolver.Name))
}

// Execute executes a GraphQL request
func (s *Schema) Execute(ctx context.Context, req *Request) *Response {
	response := &Response{}

	// Simple query parsing (production would use graphql-go library)
	operation, args, err := s.parseQuery(req.Query, req.Variables)
	if err != nil {
		response.Errors = append(response.Errors, ErrorObject{
			Message: err.Error(),
		})
		return response
	}

	// Execute query or mutation
	var result interface{}
	if query, exists := s.queries[operation]; exists {
		result, err = query.Resolve(ctx, args)
	} else if mutation, exists := s.mutations[operation]; exists {
		result, err = mutation.Resolve(ctx, args)
	} else {
		response.Errors = append(response.Errors, ErrorObject{
			Message: fmt.Sprintf("unknown operation: %s", operation),
		})
		return response
	}

	if err != nil {
		response.Errors = append(response.Errors, ErrorObject{
			Message: err.Error(),
		})
		return response
	}

	response.Data = result
	return response
}

// Handler returns HTTP handler for GraphQL endpoint
func (s *Schema) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		response := s.Execute(r.Context(), &req)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// Simple query parser (production would use proper GraphQL parser)
func (s *Schema) parseQuery(query string, variables map[string]interface{}) (string, map[string]interface{}, error) {
	// This is a simplified parser for demonstration
	// Production should use github.com/graphql-go/graphql
	
	// Extract operation name (very basic parsing)
	operation := "transaction"
	args := variables
	
	return operation, args, nil
}

// GetSchemaSDL returns GraphQL SDL
func (s *Schema) GetSchemaSDL() string {
	sdl := "type Query {\n"
	for name, query := range s.queries {
		args := ""
		for i, arg := range query.Args {
			if i > 0 {
				args += ", "
			}
			required := ""
			if arg.Required {
				required = "!"
			}
			args += fmt.Sprintf("%s: %s%s", arg.Name, arg.Type, required)
		}
		sdl += fmt.Sprintf("  %s(%s): JSON\n", name, args)
	}
	sdl += "}\n\n"

	sdl += "type Mutation {\n"
	for name, mutation := range s.mutations {
		args := ""
		for i, arg := range mutation.Args {
			if i > 0 {
				args += ", "
			}
			required := ""
			if arg.Required {
				required = "!"
			}
			args += fmt.Sprintf("%s: %s%s", arg.Name, arg.Type, required)
		}
		sdl += fmt.Sprintf("  %s(%s): JSON\n", name, args)
	}
	sdl += "}\n"

	return sdl
}

// Common resolvers for payment gateway

// TransactionQueryResolver creates transaction query resolver
func TransactionQueryResolver(fetchTransaction func(context.Context, string) (interface{}, error)) *QueryResolver {
	return &QueryResolver{
		Name:        "transaction",
		Description: "Fetch transaction by ID",
		Args: []Argument{
			{Name: "id", Type: "String", Required: true, Description: "Transaction ID"},
		},
		Resolve: func(ctx context.Context, args map[string]interface{}) (interface{}, error) {
			id, ok := args["id"].(string)
			if !ok {
				return nil, fmt.Errorf("invalid transaction ID")
			}
			return fetchTransaction(ctx, id)
		},
	}
}

// TransactionsQueryResolver creates transactions list resolver
func TransactionsQueryResolver(fetchTransactions func(context.Context, map[string]interface{}) (interface{}, error)) *QueryResolver {
	return &QueryResolver{
		Name:        "transactions",
		Description: "List transactions with filters",
		Args: []Argument{
			{Name: "limit", Type: "Int", Required: false, Description: "Number of results"},
			{Name: "offset", Type: "Int", Required: false, Description: "Offset for pagination"},
			{Name: "merchantId", Type: "String", Required: false, Description: "Filter by merchant"},
			{Name: "status", Type: "String", Required: false, Description: "Filter by status"},
			{Name: "startDate", Type: "String", Required: false, Description: "Start date filter"},
			{Name: "endDate", Type: "String", Required: false, Description: "End date filter"},
		},
		Resolve: fetchTransactions,
	}
}

// AuthorizeMutationResolver creates authorization mutation resolver
func AuthorizeMutationResolver(authorizeFunc func(context.Context, map[string]interface{}) (interface{}, error)) *MutationResolver {
	return &MutationResolver{
		Name:        "authorize",
		Description: "Authorize a transaction",
		Args: []Argument{
			{Name: "cardNumber", Type: "String", Required: true, Description: "Card number"},
			{Name: "amount", Type: "Int", Required: true, Description: "Amount in cents"},
			{Name: "currency", Type: "String", Required: true, Description: "Currency code"},
			{Name: "merchantId", Type: "String", Required: true, Description: "Merchant ID"},
			{Name: "terminalId", Type: "String", Required: true, Description: "Terminal ID"},
		},
		Resolve: authorizeFunc,
	}
}

// CaptureMutationResolver creates capture mutation resolver
func CaptureMutationResolver(captureFunc func(context.Context, map[string]interface{}) (interface{}, error)) *MutationResolver {
	return &MutationResolver{
		Name:        "capture",
		Description: "Capture a previously authorized transaction",
		Args: []Argument{
			{Name: "authorizationId", Type: "String", Required: true, Description: "Authorization ID"},
			{Name: "amount", Type: "Int", Required: false, Description: "Partial capture amount"},
		},
		Resolve: captureFunc,
	}
}

// RefundMutationResolver creates refund mutation resolver
func RefundMutationResolver(refundFunc func(context.Context, map[string]interface{}) (interface{}, error)) *MutationResolver {
	return &MutationResolver{
		Name:        "refund",
		Description: "Refund a captured transaction",
		Args: []Argument{
			{Name: "transactionId", Type: "String", Required: true, Description: "Transaction ID"},
			{Name: "amount", Type: "Int", Required: false, Description: "Partial refund amount"},
			{Name: "reason", Type: "String", Required: false, Description: "Refund reason"},
		},
		Resolve: refundFunc,
	}
}

// Subscription support (basic implementation)
type Subscription struct {
	ID        string
	Query     string
	Variables map[string]interface{}
	Channel   chan interface{}
	CreatedAt time.Time
}

// SubscriptionManager manages GraphQL subscriptions
type SubscriptionManager struct {
	subscriptions map[string]*Subscription
	mu            map[string]interface{}
	logger        *zap.Logger
}

// NewSubscriptionManager creates subscription manager
func NewSubscriptionManager(logger *zap.Logger) *SubscriptionManager {
	return &SubscriptionManager{
		subscriptions: make(map[string]*Subscription),
		mu:            make(map[string]interface{}),
		logger:        logger,
	}
}

// Subscribe creates a new subscription
func (sm *SubscriptionManager) Subscribe(query string, variables map[string]interface{}) *Subscription {
	sub := &Subscription{
		ID:        fmt.Sprintf("sub_%d", time.Now().UnixNano()),
		Query:     query,
		Variables: variables,
		Channel:   make(chan interface{}, 100),
		CreatedAt: time.Now(),
	}

	sm.subscriptions[sub.ID] = sub
	sm.logger.Info("subscription created", zap.String("id", sub.ID))

	return sub
}

// Unsubscribe removes a subscription
func (sm *SubscriptionManager) Unsubscribe(id string) {
	if sub, exists := sm.subscriptions[id]; exists {
		close(sub.Channel)
		delete(sm.subscriptions, id)
		sm.logger.Info("subscription removed", zap.String("id", id))
	}
}

// Publish publishes data to matching subscriptions
func (sm *SubscriptionManager) Publish(topic string, data interface{}) {
	for _, sub := range sm.subscriptions {
		// Simple topic matching (production would parse query)
		select {
		case sub.Channel <- data:
		default:
			sm.logger.Warn("subscription channel full", zap.String("id", sub.ID))
		}
	}
}

package logging

// contextKey is a private type to prevent key collisions in context.Context.
type contextKey string

const (
	RequestIDKey   contextKey = "request_id"
	ExecutionIDKey contextKey = "execution_id"
)

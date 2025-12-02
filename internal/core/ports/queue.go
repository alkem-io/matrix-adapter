package ports

import (
	"context"
)

// MessageHandler is the function signature for queue message handlers.
// Handlers receive the message context for cancellation, timeout, and tracing support.
type MessageHandler func(ctx context.Context, payload []byte) (interface{}, error)

// QueuePort defines the interface for message queue operations.
type QueuePort interface {
	Connect(ctx context.Context) error
	Close() error
	Publish(topic string, payload interface{}) error
	Subscribe(topic string, handler MessageHandler) error
}

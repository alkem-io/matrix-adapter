package ports

import (
	"context"
)

// MessageHandler is the function signature for queue message handlers.
// Handlers receive the message context for cancellation, timeout, and tracing support.
type MessageHandler func(ctx context.Context, payload []byte) (interface{}, error)

// QueuePort defines the interface for message queue operations.
type QueuePort interface {
	// Connect establishes a connection to the message queue.
	Connect(ctx context.Context) error
	// Close terminates the connection to the message queue.
	Close() error
	// Publish sends a message to the specified topic.
	Publish(topic string, payload interface{}) error
	// Subscribe registers a handler for messages on the specified topic.
	Subscribe(topic string, handler MessageHandler) error
}

package ports

import (
	"context"
	"time"
)

// MessageHandler is the function signature for queue message handlers.
// Handlers receive the message context for cancellation, timeout, and tracing support.
type MessageHandler func(ctx context.Context, payload []byte) (interface{}, error)

// PartitionKeyFunc extracts an ordering key from a raw message payload. Messages
// that share a key are processed strictly in order; messages with different keys
// may run concurrently. Returning an empty string routes the message to a single
// default partition (still ordered relative to other empty-key messages).
type PartitionKeyFunc func(payload []byte) string

// QueuePort defines the interface for message queue operations.
type QueuePort interface {
	// Connect establishes a connection to the message queue.
	Connect(ctx context.Context) error
	// Close terminates the connection to the message queue.
	Close() error
	// Publish sends a message to the specified topic.
	Publish(topic string, payload interface{}) error
	// Subscribe registers a handler for messages on the specified topic.
	// Messages are processed one at a time in delivery order.
	Subscribe(topic string, handler MessageHandler) error
	// SubscribeOrdered registers a handler for messages on the specified topic
	// with per-key ordering and bounded cross-key concurrency: messages sharing
	// a partition key are processed sequentially in delivery order, while
	// different keys progress concurrently up to the configured worker bound.
	// A message is acknowledged only after its handler completes.
	SubscribeOrdered(topic string, handler MessageHandler, keyFn PartitionKeyFunc) error
	// PublishAndWait sends a message to the specified topic and waits for a reply
	// using the AMQP RPC pattern (temporary exclusive reply queue + correlation_id).
	PublishAndWait(ctx context.Context, topic string, payload interface{}, timeout time.Duration) ([]byte, error)
}

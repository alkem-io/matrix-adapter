package ports

import (
	"context"
)

// QueuePort defines the interface for message queue operations.
type QueuePort interface {
	Connect(ctx context.Context) error
	Close() error
	Publish(topic string, payload interface{}) error
	Subscribe(topic string, handler func(payload []byte) (interface{}, error)) error
}

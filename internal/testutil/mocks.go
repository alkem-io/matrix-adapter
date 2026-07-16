// Package testutil provides shared test doubles for ports.QueuePort and ports.Logger.
package testutil

import (
	"context"
	"time"

	"github.com/alkem-io/matrix-adapter/internal/core/ports"
)

// MockQueuePort is a test double for ports.QueuePort that records the last published topic and payload.
type MockQueuePort struct {
	PublishedTopic         string
	PublishedPayload       interface{}
	PublishError           error
	PublishAndWaitResponse []byte
	PublishAndWaitError    error
}

var _ ports.QueuePort = (*MockQueuePort)(nil)

func (m *MockQueuePort) Connect(_ context.Context) error { return nil } //nolint:revive
func (m *MockQueuePort) Close() error                    { return nil } //nolint:revive
func (m *MockQueuePort) Publish(topic string, payload interface{}) error { //nolint:revive
	m.PublishedTopic = topic
	m.PublishedPayload = payload
	return m.PublishError
}
func (m *MockQueuePort) Subscribe(_ string, _ ports.MessageHandler) error { //nolint:revive
	return nil
}
func (m *MockQueuePort) PublishAndWait(_ context.Context, topic string, payload interface{}, _ time.Duration) ([]byte, error) { //nolint:revive
	m.PublishedTopic = topic
	m.PublishedPayload = payload
	return m.PublishAndWaitResponse, m.PublishAndWaitError
}

// MockLogger is a no-op test double for ports.Logger.
type MockLogger struct{}

var _ ports.Logger = (*MockLogger)(nil)

func (m *MockLogger) Debug(_ string, _ ...interface{})   {}           //nolint:revive
func (m *MockLogger) Info(_ string, _ ...interface{})    {}           //nolint:revive
func (m *MockLogger) Warn(_ string, _ ...interface{})    {}           //nolint:revive
func (m *MockLogger) Error(_ string, _ ...interface{})   {}           //nolint:revive
func (m *MockLogger) With(_ ...interface{}) ports.Logger { return m } //nolint:revive

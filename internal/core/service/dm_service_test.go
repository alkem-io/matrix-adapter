package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// mockQueuePort is a test double for QueuePort.
type mockQueuePort struct {
	publishedTopic   string
	publishedPayload interface{}
	publishError     error
}

func (m *mockQueuePort) Connect(_ context.Context) error { return nil }
func (m *mockQueuePort) Close() error                    { return nil }
func (m *mockQueuePort) Publish(topic string, payload interface{}) error {
	m.publishedTopic = topic
	m.publishedPayload = payload
	return m.publishError
}
func (m *mockQueuePort) Subscribe(_ string, _ ports.MessageHandler) error {
	return nil
}

// mockLogger is a test double for Logger.
type mockLogger struct{}

func (m *mockLogger) Debug(_ string, _ ...interface{})   {}
func (m *mockLogger) Info(_ string, _ ...interface{})    {}
func (m *mockLogger) Warn(_ string, _ ...interface{})    {}
func (m *mockLogger) Error(_ string, _ ...interface{})   {}
func (m *mockLogger) With(_ ...interface{}) ports.Logger { return m }

func TestDMService_PublishDMRequest_Success(t *testing.T) {
	queue := &mockQueuePort{}
	logger := &mockLogger{}
	svc := NewDMService(queue, logger)

	initiator := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	target := uuid.MustParse("660e8400-e29b-41d4-a716-446655440002")

	err := svc.PublishDMRequest(initiator, target)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.publishedTopic != dto.TopicRoomDMRequested {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomDMRequested, queue.publishedTopic)
	}

	event, ok := queue.publishedPayload.(dto.DMRequestedEvent)
	if !ok {
		t.Fatalf("expected DMRequestedEvent payload, got %T", queue.publishedPayload)
	}

	if event.InitiatorActorID != initiator.String() {
		t.Errorf("expected initiator %s, got %s", initiator.String(), event.InitiatorActorID)
	}
	if event.TargetActorID != target.String() {
		t.Errorf("expected target %s, got %s", target.String(), event.TargetActorID)
	}
}

func TestDMService_PublishDMRequest_NilInitiator(t *testing.T) {
	queue := &mockQueuePort{}
	logger := &mockLogger{}
	svc := NewDMService(queue, logger)

	target := uuid.MustParse("660e8400-e29b-41d4-a716-446655440002")

	err := svc.PublishDMRequest(uuid.Nil, target)

	if err == nil {
		t.Fatal("expected error for nil initiator")
	}

	if queue.publishedTopic != "" {
		t.Error("should not publish when initiator is nil")
	}
}

func TestDMService_PublishDMRequest_NilTarget(t *testing.T) {
	queue := &mockQueuePort{}
	logger := &mockLogger{}
	svc := NewDMService(queue, logger)

	initiator := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	err := svc.PublishDMRequest(initiator, uuid.Nil)

	if err == nil {
		t.Fatal("expected error for nil target")
	}

	if queue.publishedTopic != "" {
		t.Error("should not publish when target is nil")
	}
}

func TestDMService_PublishDMRequest_SameUser(t *testing.T) {
	queue := &mockQueuePort{}
	logger := &mockLogger{}
	svc := NewDMService(queue, logger)

	userID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	err := svc.PublishDMRequest(userID, userID)

	if err == nil {
		t.Fatal("expected error when initiator and target are the same")
	}

	if queue.publishedTopic != "" {
		t.Error("should not publish when initiator and target are the same")
	}
}

func TestDMService_PublishDMRequest_QueueError(t *testing.T) {
	queue := &mockQueuePort{
		publishError: context.DeadlineExceeded,
	}
	logger := &mockLogger{}
	svc := NewDMService(queue, logger)

	initiator := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	target := uuid.MustParse("660e8400-e29b-41d4-a716-446655440002")

	err := svc.PublishDMRequest(initiator, target)

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

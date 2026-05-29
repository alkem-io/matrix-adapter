package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/testutil"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

func TestDMService_PublishDMRequest_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	logger := &testutil.MockLogger{}
	svc := NewDMService(queue, logger)

	initiator := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	target := uuid.MustParse("660e8400-e29b-41d4-a716-446655440002")

	err := svc.PublishDMRequest(initiator, target)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicRoomDMRequested {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomDMRequested, queue.PublishedTopic)
	}

	event, ok := queue.PublishedPayload.(dto.DMRequestedEvent) //nolint:staticcheck // deprecated but kept during transition
	if !ok {
		t.Fatalf("expected DMRequestedEvent payload, got %T", queue.PublishedPayload)
	}

	if event.InitiatorActorID != initiator.String() {
		t.Errorf("expected initiator %s, got %s", initiator.String(), event.InitiatorActorID)
	}
	if event.TargetActorID != target.String() {
		t.Errorf("expected target %s, got %s", target.String(), event.TargetActorID)
	}
}

func TestDMService_PublishDMRequest_NilInitiator(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	logger := &testutil.MockLogger{}
	svc := NewDMService(queue, logger)

	target := uuid.MustParse("660e8400-e29b-41d4-a716-446655440002")

	err := svc.PublishDMRequest(uuid.Nil, target)

	if err == nil {
		t.Fatal("expected error for nil initiator")
	}

	if queue.PublishedTopic != "" {
		t.Error("should not publish when initiator is nil")
	}
}

func TestDMService_PublishDMRequest_NilTarget(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	logger := &testutil.MockLogger{}
	svc := NewDMService(queue, logger)

	initiator := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	err := svc.PublishDMRequest(initiator, uuid.Nil)

	if err == nil {
		t.Fatal("expected error for nil target")
	}

	if queue.PublishedTopic != "" {
		t.Error("should not publish when target is nil")
	}
}

func TestDMService_PublishDMRequest_SameUser(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	logger := &testutil.MockLogger{}
	svc := NewDMService(queue, logger)

	userID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

	err := svc.PublishDMRequest(userID, userID)

	if err == nil {
		t.Fatal("expected error when initiator and target are the same")
	}

	if queue.PublishedTopic != "" {
		t.Error("should not publish when initiator and target are the same")
	}
}

func TestDMService_PublishDMRequest_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{
		PublishError: context.DeadlineExceeded,
	}
	logger := &testutil.MockLogger{}
	svc := NewDMService(queue, logger)

	initiator := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	target := uuid.MustParse("660e8400-e29b-41d4-a716-446655440002")

	err := svc.PublishDMRequest(initiator, target)

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
	"github.com/alkem-io/matrix-adapter-go/internal/testutil"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
)

// newEventService creates an EventService with the given mock queue for testing.
// Config is nil because no handler reads it.
func newEventService(queue *testutil.MockQueuePort) *EventService {
	return NewEventService(queue, &testutil.MockLogger{}, nil)
}

// ── HandleMessage ──────────────────────────────────────────────────────────

func TestHandleMessage_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	senderID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	msg := domain.Message{
		ID:        "$event1:example.com",
		RoomID:    "!room1:example.com",
		RoomName:  "Test Room",
		SenderID:  senderID,
		Content:   "Hello, world!",
		Timestamp: now,
	}

	if err := svc.HandleMessage(msg); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicMessageReceived {
		t.Errorf("expected topic %s, got %s", dto.TopicMessageReceived, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.MessageReceivedPayload)
	if !ok {
		t.Fatalf("expected MessageReceivedPayload, got %T", queue.PublishedPayload)
	}

	if payload.RoomID != "!room1:example.com" {
		t.Errorf("expected RoomID !room1:example.com, got %s", payload.RoomID)
	}
	if payload.RoomName != "Test Room" {
		t.Errorf("expected RoomName 'Test Room', got %s", payload.RoomName)
	}
	if payload.ActorID != senderID.String() {
		t.Errorf("expected ActorID %s, got %s", senderID, payload.ActorID)
	}
	if payload.Message.ID != "$event1:example.com" {
		t.Errorf("expected Message.ID $event1:example.com, got %s", payload.Message.ID)
	}
	if payload.Message.Message != "Hello, world!" {
		t.Errorf("expected Message.Message 'Hello, world!', got %s", payload.Message.Message)
	}
	if payload.Message.Sender != senderID.String() {
		t.Errorf("expected Message.Sender %s, got %s", senderID, payload.Message.Sender)
	}
	if payload.Message.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Message.Timestamp)
	}
	if payload.Message.ThreadID != nil {
		t.Errorf("expected nil ThreadID, got %v", payload.Message.ThreadID)
	}
}

func TestHandleMessage_WithThreadID(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	msg := domain.Message{
		ID:        "$event2:example.com",
		RoomID:    "!room1:example.com",
		RoomName:  "Test Room",
		SenderID:  uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Content:   "Thread reply",
		Timestamp: time.Now(),
		ThreadID:  "$thread-root:example.com",
	}

	if err := svc.HandleMessage(msg); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	payload := queue.PublishedPayload.(dto.MessageReceivedPayload)
	if payload.Message.ThreadID == nil {
		t.Fatal("expected non-nil ThreadID")
	}
	if string(*payload.Message.ThreadID) != "$thread-root:example.com" {
		t.Errorf("expected ThreadID $thread-root:example.com, got %s", *payload.Message.ThreadID)
	}
}

func TestHandleMessage_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleMessage(domain.Message{
		ID:        "$ev:example.com",
		RoomID:    "!r:example.com",
		SenderID:  uuid.New(),
		Timestamp: time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleReactionAdded ────────────────────────────────────────────────────

func TestHandleReactionAdded_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	senderID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")

	evt := domain.ReactionEvent{
		AlkemioRoomID: roomID,
		MessageID:     id.EventID("$msg1:example.com"),
		ReactionID:    id.EventID("$react1:example.com"),
		Emoji:         "👍",
		SenderActorID: senderID,
		Timestamp:     now,
	}

	if err := svc.HandleReactionAdded(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicReactionAdded {
		t.Errorf("expected topic %s, got %s", dto.TopicReactionAdded, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.ReactionAddedEvent)
	if !ok {
		t.Fatalf("expected ReactionAddedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if string(payload.MessageID) != "$msg1:example.com" {
		t.Errorf("expected MessageID $msg1:example.com, got %s", payload.MessageID)
	}
	if string(payload.ReactionID) != "$react1:example.com" {
		t.Errorf("expected ReactionID $react1:example.com, got %s", payload.ReactionID)
	}
	if payload.Emoji != "👍" {
		t.Errorf("expected Emoji 👍, got %s", payload.Emoji)
	}
	if uuid.UUID(payload.SenderActorID) != senderID {
		t.Errorf("expected SenderActorID %s, got %s", senderID, payload.SenderActorID)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleReactionAdded_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleReactionAdded(domain.ReactionEvent{
		AlkemioRoomID: uuid.New(),
		MessageID:     id.EventID("$m:x"),
		ReactionID:    id.EventID("$r:x"),
		SenderActorID: uuid.New(),
		Timestamp:     time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleReactionRemoved ──────────────────────────────────────────────────

func TestHandleReactionRemoved_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
	senderID := uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")

	evt := domain.ReactionRemovedEvent{
		AlkemioRoomID: roomID,
		MessageID:     id.EventID("$msg2:example.com"),
		ReactionID:    id.EventID("$react2:example.com"),
		Emoji:         "❤️",
		SenderActorID: senderID,
		Timestamp:     now,
	}

	if err := svc.HandleReactionRemoved(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicReactionRemoved {
		t.Errorf("expected topic %s, got %s", dto.TopicReactionRemoved, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.ReactionRemovedEvent)
	if !ok {
		t.Fatalf("expected ReactionRemovedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if string(payload.MessageID) != "$msg2:example.com" {
		t.Errorf("expected MessageID $msg2:example.com, got %s", payload.MessageID)
	}
	if string(payload.ReactionID) != "$react2:example.com" {
		t.Errorf("expected ReactionID $react2:example.com, got %s", payload.ReactionID)
	}
	if payload.Emoji != "❤️" {
		t.Errorf("expected Emoji, got %s", payload.Emoji)
	}
	if uuid.UUID(payload.SenderActorID) != senderID {
		t.Errorf("expected SenderActorID %s, got %s", senderID, payload.SenderActorID)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleReactionRemoved_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleReactionRemoved(domain.ReactionRemovedEvent{
		AlkemioRoomID: uuid.New(),
		MessageID:     id.EventID("$m:x"),
		ReactionID:    id.EventID("$r:x"),
		SenderActorID: uuid.New(),
		Timestamp:     time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleMemberLeft ───────────────────────────────────────────────────────

func TestHandleMemberLeft_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	actorID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	evt := domain.MembershipEvent{
		AlkemioRoomID: roomID,
		ActorID:       actorID,
		Reason:        "left voluntarily",
		Timestamp:     now,
	}

	if err := svc.HandleMemberLeft(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicRoomMemberLeft {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomMemberLeft, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.RoomMemberLeftEvent)
	if !ok {
		t.Fatalf("expected RoomMemberLeftEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if uuid.UUID(payload.ActorID) != actorID {
		t.Errorf("expected ActorID %s, got %s", actorID, payload.ActorID)
	}
	if payload.Reason != "left voluntarily" {
		t.Errorf("expected Reason 'left voluntarily', got %s", payload.Reason)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleMemberLeft_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleMemberLeft(domain.MembershipEvent{
		AlkemioRoomID: uuid.New(),
		ActorID:       uuid.New(),
		Timestamp:     time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleReadReceiptUpdated ───────────────────────────────────────────────

func TestHandleReadReceiptUpdated_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	userID := uuid.MustParse("44444444-4444-4444-4444-444444444444")

	evt := domain.ReadReceiptEvent{
		AlkemioRoomID: roomID,
		UserID:        userID,
		EventID:       id.EventID("$read1:example.com"),
		Timestamp:     now,
	}

	if err := svc.HandleReadReceiptUpdated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicReadReceiptUpdated {
		t.Errorf("expected topic %s, got %s", dto.TopicReadReceiptUpdated, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.ReadReceiptUpdatedEvent)
	if !ok {
		t.Fatalf("expected ReadReceiptUpdatedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if uuid.UUID(payload.ActorID) != userID {
		t.Errorf("expected ActorID %s, got %s", userID, payload.ActorID)
	}
	if string(payload.EventID) != "$read1:example.com" {
		t.Errorf("expected EventID $read1:example.com, got %s", payload.EventID)
	}
	if payload.ThreadID != nil {
		t.Errorf("expected nil ThreadID, got %v", payload.ThreadID)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleReadReceiptUpdated_WithThreadID(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	threadRoot := id.EventID("$thread-root:example.com")
	evt := domain.ReadReceiptEvent{
		AlkemioRoomID: uuid.New(),
		UserID:        uuid.New(),
		EventID:       id.EventID("$read2:example.com"),
		ThreadID:      &threadRoot,
		Timestamp:     time.Now(),
	}

	if err := svc.HandleReadReceiptUpdated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	payload := queue.PublishedPayload.(dto.ReadReceiptUpdatedEvent)
	if payload.ThreadID == nil {
		t.Fatal("expected non-nil ThreadID")
	}
	if string(*payload.ThreadID) != "$thread-root:example.com" {
		t.Errorf("expected ThreadID $thread-root:example.com, got %s", *payload.ThreadID)
	}
}

func TestHandleReadReceiptUpdated_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleReadReceiptUpdated(domain.ReadReceiptEvent{
		AlkemioRoomID: uuid.New(),
		UserID:        uuid.New(),
		EventID:       id.EventID("$e:x"),
		Timestamp:     time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleMessageEdited ────────────────────────────────────────────────────

func TestHandleMessageEdited_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	senderID := uuid.MustParse("66666666-6666-6666-6666-666666666666")

	evt := domain.MessageEditedEvent{
		AlkemioRoomID:   roomID,
		OriginalEventID: id.EventID("$orig:example.com"),
		NewEventID:      id.EventID("$edit:example.com"),
		SenderID:        senderID,
		NewContent:      "Updated message",
		Timestamp:       now,
	}

	if err := svc.HandleMessageEdited(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicMessageEdited {
		t.Errorf("expected topic %s, got %s", dto.TopicMessageEdited, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.MessageEditedEvent)
	if !ok {
		t.Fatalf("expected MessageEditedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if uuid.UUID(payload.SenderActorID) != senderID {
		t.Errorf("expected SenderActorID %s, got %s", senderID, payload.SenderActorID)
	}
	if string(payload.OriginalMessageID) != "$orig:example.com" {
		t.Errorf("expected OriginalMessageID $orig:example.com, got %s", payload.OriginalMessageID)
	}
	if string(payload.NewMessageID) != "$edit:example.com" {
		t.Errorf("expected NewMessageID $edit:example.com, got %s", payload.NewMessageID)
	}
	if payload.NewContent != "Updated message" {
		t.Errorf("expected NewContent 'Updated message', got %s", payload.NewContent)
	}
	if payload.ThreadID != nil {
		t.Errorf("expected nil ThreadID, got %v", payload.ThreadID)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleMessageEdited_WithThreadID(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	threadRoot := id.EventID("$thread-root:example.com")
	evt := domain.MessageEditedEvent{
		AlkemioRoomID:   uuid.New(),
		OriginalEventID: id.EventID("$orig:x"),
		NewEventID:      id.EventID("$edit:x"),
		SenderID:        uuid.New(),
		NewContent:      "edited",
		ThreadID:        &threadRoot,
		Timestamp:       time.Now(),
	}

	if err := svc.HandleMessageEdited(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	payload := queue.PublishedPayload.(dto.MessageEditedEvent)
	if payload.ThreadID == nil {
		t.Fatal("expected non-nil ThreadID")
	}
	if string(*payload.ThreadID) != "$thread-root:example.com" {
		t.Errorf("expected ThreadID $thread-root:example.com, got %s", *payload.ThreadID)
	}
}

func TestHandleMessageEdited_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleMessageEdited(domain.MessageEditedEvent{
		AlkemioRoomID:   uuid.New(),
		OriginalEventID: id.EventID("$o:x"),
		NewEventID:      id.EventID("$n:x"),
		SenderID:        uuid.New(),
		Timestamp:       time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleMessageRedacted ──────────────────────────────────────────────────

func TestHandleMessageRedacted_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	redactorID := uuid.MustParse("88888888-8888-8888-8888-888888888888")

	evt := domain.MessageRedactedEvent{
		AlkemioRoomID:    roomID,
		RedactedEventID:  id.EventID("$redacted:example.com"),
		RedactionEventID: id.EventID("$redaction:example.com"),
		RedactorID:       redactorID,
		Reason:           "spam",
		Timestamp:        now,
	}

	if err := svc.HandleMessageRedacted(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicMessageRedacted {
		t.Errorf("expected topic %s, got %s", dto.TopicMessageRedacted, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.MessageRedactedEvent)
	if !ok {
		t.Fatalf("expected MessageRedactedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if uuid.UUID(payload.RedactorActorID) != redactorID {
		t.Errorf("expected RedactorActorID %s, got %s", redactorID, payload.RedactorActorID)
	}
	if string(payload.RedactedMessageID) != "$redacted:example.com" {
		t.Errorf("expected RedactedMessageID $redacted:example.com, got %s", payload.RedactedMessageID)
	}
	if string(payload.RedactionMessageID) != "$redaction:example.com" {
		t.Errorf("expected RedactionMessageID $redaction:example.com, got %s", payload.RedactionMessageID)
	}
	if payload.Reason != "spam" {
		t.Errorf("expected Reason 'spam', got %s", payload.Reason)
	}
	if payload.ThreadID != nil {
		t.Errorf("expected nil ThreadID, got %v", payload.ThreadID)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleMessageRedacted_WithThreadID(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	threadRoot := id.EventID("$thread-root:example.com")
	evt := domain.MessageRedactedEvent{
		AlkemioRoomID:    uuid.New(),
		RedactedEventID:  id.EventID("$rd:x"),
		RedactionEventID: id.EventID("$rn:x"),
		RedactorID:       uuid.New(),
		ThreadID:         &threadRoot,
		Timestamp:        time.Now(),
	}

	if err := svc.HandleMessageRedacted(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	payload := queue.PublishedPayload.(dto.MessageRedactedEvent)
	if payload.ThreadID == nil {
		t.Fatal("expected non-nil ThreadID")
	}
	if string(*payload.ThreadID) != "$thread-root:example.com" {
		t.Errorf("expected ThreadID $thread-root:example.com, got %s", *payload.ThreadID)
	}
}

func TestHandleMessageRedacted_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleMessageRedacted(domain.MessageRedactedEvent{
		AlkemioRoomID:    uuid.New(),
		RedactedEventID:  id.EventID("$rd:x"),
		RedactionEventID: id.EventID("$rn:x"),
		RedactorID:       uuid.New(),
		Timestamp:        time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleRoomCreated ──────────────────────────────────────────────────────

func TestHandleRoomCreated_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("99999999-9999-9999-9999-999999999999")
	creatorID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")

	evt := domain.RoomCreatedEvent{
		AlkemioRoomID: roomID,
		MatrixRoomID:  id.RoomID("!created:example.com"),
		CreatorID:     creatorID,
		RoomType:      "standard",
		Name:          "New Room",
		Topic:         "Room topic",
		Timestamp:     now,
	}

	if err := svc.HandleRoomCreated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicRoomCreated {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomCreated, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.RoomCreatedEvent)
	if !ok {
		t.Fatalf("expected RoomCreatedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if uuid.UUID(payload.CreatorActorID) != creatorID {
		t.Errorf("expected CreatorActorID %s, got %s", creatorID, payload.CreatorActorID)
	}
	if payload.RoomType != "standard" {
		t.Errorf("expected RoomType 'standard', got %s", payload.RoomType)
	}
	if payload.Name != "New Room" {
		t.Errorf("expected Name 'New Room', got %s", payload.Name)
	}
	if payload.Topic != "Room topic" {
		t.Errorf("expected Topic 'Room topic', got %s", payload.Topic)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleRoomCreated_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleRoomCreated(domain.RoomCreatedEvent{
		AlkemioRoomID: uuid.New(),
		CreatorID:     uuid.New(),
		Timestamp:     time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleRoomUpdated ──────────────────────────────────────────────────────

func TestHandleRoomUpdated_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("abababab-abab-abab-abab-abababababab")
	displayName := "Updated Name"
	avatarURL := "mxc://example.com/avatar"
	topic := "Updated topic"

	evt := domain.RoomUpdatedEvent{
		AlkemioRoomID: roomID,
		DisplayName:   &displayName,
		AvatarURL:     &avatarURL,
		Topic:         &topic,
		Timestamp:     now,
	}

	if err := svc.HandleRoomUpdated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicRoomUpdated {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomUpdated, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.RoomUpdatedEvent)
	if !ok {
		t.Fatalf("expected RoomUpdatedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if payload.DisplayName == nil || *payload.DisplayName != "Updated Name" {
		t.Errorf("expected DisplayName 'Updated Name', got %v", payload.DisplayName)
	}
	if payload.AvatarURL == nil || *payload.AvatarURL != "mxc://example.com/avatar" {
		t.Errorf("expected AvatarURL 'mxc://example.com/avatar', got %v", payload.AvatarURL)
	}
	if payload.Topic == nil || *payload.Topic != "Updated topic" {
		t.Errorf("expected Topic 'Updated topic', got %v", payload.Topic)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleRoomUpdated_NilFields(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	evt := domain.RoomUpdatedEvent{
		AlkemioRoomID: uuid.New(),
		Timestamp:     time.Now(),
	}

	if err := svc.HandleRoomUpdated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	payload := queue.PublishedPayload.(dto.RoomUpdatedEvent)
	if payload.DisplayName != nil {
		t.Errorf("expected nil DisplayName, got %v", payload.DisplayName)
	}
	if payload.AvatarURL != nil {
		t.Errorf("expected nil AvatarURL, got %v", payload.AvatarURL)
	}
	if payload.Topic != nil {
		t.Errorf("expected nil Topic, got %v", payload.Topic)
	}
}

func TestHandleRoomUpdated_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleRoomUpdated(domain.RoomUpdatedEvent{
		AlkemioRoomID: uuid.New(),
		Timestamp:     time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleSpaceUpdated ─────────────────────────────────────────────────────

func TestHandleSpaceUpdated_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	contextID := uuid.MustParse("cdcdcdcd-cdcd-cdcd-cdcd-cdcdcdcdcdcd")
	displayName := "Space Name"
	avatarURL := "mxc://example.com/space-avatar"
	topic := "Space topic"

	evt := domain.SpaceUpdatedEvent{
		AlkemioContextID: contextID,
		DisplayName:      &displayName,
		AvatarURL:        &avatarURL,
		Topic:            &topic,
		Timestamp:        now,
	}

	if err := svc.HandleSpaceUpdated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicSpaceUpdated {
		t.Errorf("expected topic %s, got %s", dto.TopicSpaceUpdated, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.SpaceUpdatedEvent)
	if !ok {
		t.Fatalf("expected SpaceUpdatedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioContextID) != contextID {
		t.Errorf("expected AlkemioContextID %s, got %s", contextID, payload.AlkemioContextID)
	}
	if payload.DisplayName == nil || *payload.DisplayName != "Space Name" {
		t.Errorf("expected DisplayName 'Space Name', got %v", payload.DisplayName)
	}
	if payload.AvatarURL == nil || *payload.AvatarURL != "mxc://example.com/space-avatar" {
		t.Errorf("expected AvatarURL 'mxc://example.com/space-avatar', got %v", payload.AvatarURL)
	}
	if payload.Topic == nil || *payload.Topic != "Space topic" {
		t.Errorf("expected Topic 'Space topic', got %v", payload.Topic)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleSpaceUpdated_NilFields(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	evt := domain.SpaceUpdatedEvent{
		AlkemioContextID: uuid.New(),
		Timestamp:        time.Now(),
	}

	if err := svc.HandleSpaceUpdated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	payload := queue.PublishedPayload.(dto.SpaceUpdatedEvent)
	if payload.DisplayName != nil {
		t.Errorf("expected nil DisplayName, got %v", payload.DisplayName)
	}
	if payload.AvatarURL != nil {
		t.Errorf("expected nil AvatarURL, got %v", payload.AvatarURL)
	}
	if payload.Topic != nil {
		t.Errorf("expected nil Topic, got %v", payload.Topic)
	}
}

func TestHandleSpaceUpdated_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleSpaceUpdated(domain.SpaceUpdatedEvent{
		AlkemioContextID: uuid.New(),
		Timestamp:        time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

// ── HandleMemberUpdated ────────────────────────────────────────────────────

func TestHandleMemberUpdated_Success(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newEventService(queue)

	now := time.Now()
	roomID := uuid.MustParse("efefefef-efef-efef-efef-efefefefefef")
	memberID := uuid.MustParse("12121212-1212-1212-1212-121212121212")
	senderID := uuid.MustParse("34343434-3434-3434-3434-343434343434")

	evt := domain.RoomMemberUpdatedEvent{
		AlkemioRoomID: roomID,
		MemberID:      memberID,
		SenderID:      senderID,
		Membership:    "join",
		Timestamp:     now,
	}

	if err := svc.HandleMemberUpdated(evt); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if queue.PublishedTopic != dto.TopicRoomMemberUpdated {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomMemberUpdated, queue.PublishedTopic)
	}

	payload, ok := queue.PublishedPayload.(dto.RoomMemberUpdatedEvent)
	if !ok {
		t.Fatalf("expected RoomMemberUpdatedEvent, got %T", queue.PublishedPayload)
	}

	if uuid.UUID(payload.AlkemioRoomID) != roomID {
		t.Errorf("expected AlkemioRoomID %s, got %s", roomID, payload.AlkemioRoomID)
	}
	if uuid.UUID(payload.MemberActorID) != memberID {
		t.Errorf("expected MemberActorID %s, got %s", memberID, payload.MemberActorID)
	}
	if uuid.UUID(payload.SenderActorID) != senderID {
		t.Errorf("expected SenderActorID %s, got %s", senderID, payload.SenderActorID)
	}
	if payload.Membership != "join" {
		t.Errorf("expected Membership 'join', got %s", payload.Membership)
	}
	if payload.Timestamp != now.UnixMilli() {
		t.Errorf("expected Timestamp %d, got %d", now.UnixMilli(), payload.Timestamp)
	}
}

func TestHandleMemberUpdated_QueueError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishError: context.DeadlineExceeded}
	svc := newEventService(queue)

	err := svc.HandleMemberUpdated(domain.RoomMemberUpdatedEvent{
		AlkemioRoomID: uuid.New(),
		MemberID:      uuid.New(),
		SenderID:      uuid.New(),
		Membership:    "invite",
		Timestamp:     time.Now(),
	})

	if err == nil {
		t.Fatal("expected error when queue fails")
	}
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

const (
	checkCreatorUUID = "550e8400-e29b-41d4-a716-446655440001"
	checkMemberUUID  = "660e8400-e29b-41d4-a716-446655440002"
	checkHSDomain    = "matrix.alkemio.org"
	checkRoomUUID    = "770e8400-e29b-41d4-a716-446655440003"
)

func newCheckService(queue *testutil.MockQueuePort) *RoomCheckService {
	logger := &testutil.MockLogger{}
	idMapper := domain.NewIDMapper(checkHSDomain)
	return NewRoomCheckService(queue, idMapper, logger)
}

func TestCheckRoom_Allow(t *testing.T) {
	resp := dto.CheckRoomResponse{Allow: true, AlkemioRoomID: checkRoomUUID}
	respBytes, _ := json.Marshal(resp)

	queue := &testutil.MockQueuePort{PublishAndWaitResponse: respBytes}
	svc := newCheckService(queue)

	req := domain.RoomCheckRequest{
		Creator:  id.UserID("@" + checkCreatorUUID + ":" + checkHSDomain),
		Members:  []id.UserID{id.UserID("@" + checkMemberUUID + ":" + checkHSDomain)},
		IsDirect: true,
	}

	result, err := svc.CheckRoom(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !result.Allow {
		t.Error("expected allow=true")
	}
	if result.AlkemioRoomID != checkRoomUUID {
		t.Errorf("expected alkemioRoomID %s, got %s", checkRoomUUID, result.AlkemioRoomID)
	}
	if queue.PublishedTopic != dto.TopicRoomCheck {
		t.Errorf("expected topic %s, got %s", dto.TopicRoomCheck, queue.PublishedTopic)
	}
}

func TestCheckRoom_Reject(t *testing.T) {
	resp := dto.CheckRoomResponse{Allow: false, Reason: "no consent"}
	respBytes, _ := json.Marshal(resp)

	queue := &testutil.MockQueuePort{PublishAndWaitResponse: respBytes}
	svc := newCheckService(queue)

	req := domain.RoomCheckRequest{
		Creator:  id.UserID("@" + checkCreatorUUID + ":" + checkHSDomain),
		Members:  []id.UserID{id.UserID("@" + checkMemberUUID + ":" + checkHSDomain)},
		IsDirect: true,
	}

	result, err := svc.CheckRoom(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result.Allow {
		t.Error("expected allow=false")
	}
	if result.Reason != "no consent" {
		t.Errorf("expected reason 'no consent', got %q", result.Reason)
	}
}

func TestCheckRoom_InvalidCreatorID(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newCheckService(queue)

	req := domain.RoomCheckRequest{
		Creator:  id.UserID("@not-a-uuid:" + checkHSDomain),
		Members:  []id.UserID{id.UserID("@" + checkMemberUUID + ":" + checkHSDomain)},
		IsDirect: false,
	}

	_, err := svc.CheckRoom(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid creator ID")
	}
}

func TestCheckRoom_InvalidMemberID(t *testing.T) {
	queue := &testutil.MockQueuePort{}
	svc := newCheckService(queue)

	req := domain.RoomCheckRequest{
		Creator:  id.UserID("@" + checkCreatorUUID + ":" + checkHSDomain),
		Members:  []id.UserID{id.UserID("@bad-id:" + checkHSDomain)},
		IsDirect: false,
	}

	_, err := svc.CheckRoom(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid member ID")
	}
}

func TestCheckRoom_PublishAndWaitError(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishAndWaitError: fmt.Errorf("timeout")}
	svc := newCheckService(queue)

	req := domain.RoomCheckRequest{
		Creator:  id.UserID("@" + checkCreatorUUID + ":" + checkHSDomain),
		Members:  []id.UserID{id.UserID("@" + checkMemberUUID + ":" + checkHSDomain)},
		IsDirect: true,
	}

	_, err := svc.CheckRoom(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when PublishAndWait fails")
	}
}

func TestCheckRoom_MalformedResponse(t *testing.T) {
	queue := &testutil.MockQueuePort{PublishAndWaitResponse: []byte("not json")}
	svc := newCheckService(queue)

	req := domain.RoomCheckRequest{
		Creator:  id.UserID("@" + checkCreatorUUID + ":" + checkHSDomain),
		Members:  []id.UserID{id.UserID("@" + checkMemberUUID + ":" + checkHSDomain)},
		IsDirect: true,
	}

	_, err := svc.CheckRoom(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for malformed JSON response")
	}
}

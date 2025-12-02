package dto

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestAlkemioRoomID_JSONRoundTrip(t *testing.T) {
	original := uuid.New()
	roomID := AlkemioRoomID(original)

	// Marshal
	data, err := json.Marshal(roomID)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Should be a quoted UUID string
	expected := `"` + original.String() + `"`
	if string(data) != expected {
		t.Errorf("Marshal: got %s, want %s", string(data), expected)
	}

	// Unmarshal
	var decoded AlkemioRoomID
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.UUID() != original {
		t.Errorf("Unmarshal: got %v, want %v", decoded.UUID(), original)
	}
}

func TestAlkemioActorID_JSONRoundTrip(t *testing.T) {
	original := uuid.New()
	actorID := AlkemioActorID(original)

	data, err := json.Marshal(actorID)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded AlkemioActorID
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.UUID() != original {
		t.Errorf("Unmarshal: got %v, want %v", decoded.UUID(), original)
	}
}

func TestAlkemioContextID_JSONRoundTrip(t *testing.T) {
	original := uuid.New()
	contextID := AlkemioContextID(original)

	data, err := json.Marshal(contextID)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded AlkemioContextID
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.UUID() != original {
		t.Errorf("Unmarshal: got %v, want %v", decoded.UUID(), original)
	}
}

func TestAlkemioRoomID_String(t *testing.T) {
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	roomID := AlkemioRoomID(id)

	if roomID.String() != id.String() {
		t.Errorf("String: got %s, want %s", roomID.String(), id.String())
	}
}

func TestAlkemioRoomID_UnmarshalInvalidJSON(t *testing.T) {
	var roomID AlkemioRoomID
	err := json.Unmarshal([]byte(`"not-a-uuid"`), &roomID)
	if err == nil {
		t.Error("Expected error for invalid UUID, got nil")
	}
}

func TestCreateRoomRequest_JSONUnmarshal(t *testing.T) {
	jsonData := `{
		"alkemio_room_id": "550e8400-e29b-41d4-a716-446655440000",
		"type": "community",
		"name": "Test Room",
		"initial_members": ["660e8400-e29b-41d4-a716-446655440001", "770e8400-e29b-41d4-a716-446655440002"],
		"join_rule": "invite"
	}`

	var req CreateRoomRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	expectedRoomID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	if req.AlkemioRoomID.UUID() != expectedRoomID {
		t.Errorf("AlkemioRoomID: got %v, want %v", req.AlkemioRoomID.UUID(), expectedRoomID)
	}

	if req.Type != RoomTypeCommunity {
		t.Errorf("Type: got %s, want %s", req.Type, RoomTypeCommunity)
	}

	if len(req.InitialMembers) != 2 {
		t.Errorf("InitialMembers: got %d members, want 2", len(req.InitialMembers))
	}

	if req.JoinRule != JoinRuleInvite {
		t.Errorf("JoinRule: got %s, want %s", req.JoinRule, JoinRuleInvite)
	}
}

func TestBaseResponse_JSONMarshal(t *testing.T) {
	// Success response
	success := NewSuccessResponse()
	data, err := json.Marshal(success)
	if err != nil {
		t.Fatalf("Marshal success failed: %v", err)
	}

	var decoded BaseResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal success failed: %v", err)
	}
	if !decoded.Success {
		t.Error("Expected success=true")
	}
	if decoded.Error != nil {
		t.Error("Expected error=nil for success response")
	}

	// Error response
	errResp := NewErrorResponse(ErrCodeRoomNotFound, "room xyz not found")
	data, err = json.Marshal(errResp)
	if err != nil {
		t.Fatalf("Marshal error failed: %v", err)
	}

	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error failed: %v", err)
	}
	if decoded.Success {
		t.Error("Expected success=false")
	}
	if decoded.Error == nil {
		t.Fatal("Expected error!=nil")
	}
	if decoded.Error.Code != ErrCodeRoomNotFound {
		t.Errorf("Error.Code: got %s, want %s", decoded.Error.Code, ErrCodeRoomNotFound)
	}
}

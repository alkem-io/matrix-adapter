//nolint:errcheck,revive // test file — unchecked writes and unused params are acceptable
package matrix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// newTestSynapseAdmin creates a SynapseAdmin backed by the given httptest.Server.
func newTestSynapseAdmin(t *testing.T, server *httptest.Server) *SynapseAdmin {
	t.Helper()
	sa, err := NewSynapseAdmin(server.URL, "test-token")
	if err != nil {
		t.Fatalf("NewSynapseAdmin: %v", err)
	}
	return sa
}

// --------------------------------------------------------------------------
// Constructor
// --------------------------------------------------------------------------

func TestNewSynapseAdmin_Success(t *testing.T) {
	sa, err := NewSynapseAdmin("http://localhost:8008", "tok")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sa == nil {
		t.Fatal("expected non-nil SynapseAdmin")
	}
}

func TestNewSynapseAdmin_InvalidURL(t *testing.T) {
	_, err := NewSynapseAdmin("://bad-url", "tok")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// --------------------------------------------------------------------------
// GetUser
// --------------------------------------------------------------------------

func TestGetUser_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/_synapse/admin/v2/users/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"admin": true})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	info, err := sa.GetUser(context.Background(), "@alice:example.com")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !info.Admin {
		t.Error("expected admin=true")
	}
}

func TestGetUser_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_NOT_FOUND",
			"error":   "User not found",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetUser(context.Background(), "@missing:example.com")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// --------------------------------------------------------------------------
// SetUserAdmin
// --------------------------------------------------------------------------

func TestSetUserAdmin_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.SetUserAdmin(context.Background(), "@alice:example.com", true)
	if err != nil {
		t.Fatalf("SetUserAdmin: %v", err)
	}
	if admin, ok := receivedBody["admin"].(bool); !ok || !admin {
		t.Errorf("expected admin=true in request body, got %v", receivedBody)
	}
}

func TestSetUserAdmin_False(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.SetUserAdmin(context.Background(), "@alice:example.com", false)
	if err != nil {
		t.Fatalf("SetUserAdmin: %v", err)
	}
	if admin, ok := receivedBody["admin"].(bool); !ok || admin {
		t.Errorf("expected admin=false in request body, got %v", receivedBody)
	}
}

func TestSetUserAdmin_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_FORBIDDEN",
			"error":   "Not an admin",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.SetUserAdmin(context.Background(), "@alice:example.com", true)
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// --------------------------------------------------------------------------
// DeactivateUser
// --------------------------------------------------------------------------

func TestDeactivateUser_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/_synapse/admin/v1/deactivate/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.DeactivateUser(context.Background(), "@alice:example.com", true)
	if err != nil {
		t.Fatalf("DeactivateUser: %v", err)
	}
	if erase, ok := receivedBody["erase"].(bool); !ok || !erase {
		t.Errorf("expected erase=true in request body, got %v", receivedBody)
	}
}

func TestDeactivateUser_NoErase(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.DeactivateUser(context.Background(), "@alice:example.com", false)
	if err != nil {
		t.Fatalf("DeactivateUser: %v", err)
	}
	if erase, ok := receivedBody["erase"].(bool); !ok || erase {
		t.Errorf("expected erase=false in request body, got %v", receivedBody)
	}
}

func TestDeactivateUser_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_UNKNOWN",
			"error":   "Internal error",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.DeactivateUser(context.Background(), "@alice:example.com", false)
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// --------------------------------------------------------------------------
// ListRooms
// --------------------------------------------------------------------------

func TestListRooms_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/_synapse/admin/v1/rooms") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("expected limit=10, got %s", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"rooms": []map[string]interface{}{
				{"room_id": "!room1:hs", "room_type": "", "canonical_alias": "#room1:hs"},
				{"room_id": "!room2:hs", "room_type": "m.space", "canonical_alias": "#space1:hs"},
			},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	rooms, err := sa.ListRooms(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListRooms: %v", err)
	}
	if len(rooms) != 2 {
		t.Fatalf("expected 2 rooms, got %d", len(rooms))
	}
	if rooms[0].RoomID != "!room1:hs" {
		t.Errorf("expected room_id=!room1:hs, got %s", rooms[0].RoomID)
	}
	if rooms[1].RoomType != "m.space" {
		t.Errorf("expected room_type=m.space, got %s", rooms[1].RoomType)
	}
	if rooms[0].CanonicalAlias != "#room1:hs" {
		t.Errorf("expected canonical_alias=#room1:hs, got %s", rooms[0].CanonicalAlias)
	}
}

func TestListRooms_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_FORBIDDEN",
			"error":   "Not an admin",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.ListRooms(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// --------------------------------------------------------------------------
// GetRoomMembers
// --------------------------------------------------------------------------

func TestGetRoomMembers_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// Path has %21 for '!' in room ID
		if !strings.Contains(r.URL.RawPath, "/members") && !strings.Contains(r.URL.Path, "/members") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []string{"@alice:hs", "@bob:hs"},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	members, err := sa.GetRoomMembers(context.Background(), "!room1:hs")
	if err != nil {
		t.Fatalf("GetRoomMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
	if members[0] != "@alice:hs" {
		t.Errorf("expected @alice:hs, got %s", members[0])
	}
	if members[1] != "@bob:hs" {
		t.Errorf("expected @bob:hs, got %s", members[1])
	}
}

func TestGetRoomMembers_EmptyRoom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []string{},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	members, err := sa.GetRoomMembers(context.Background(), "!room1:hs")
	if err != nil {
		t.Fatalf("GetRoomMembers: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

func TestGetRoomMembers_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_NOT_FOUND",
			"error":   "Room not found",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRoomMembers(context.Background(), "!missing:hs")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// --------------------------------------------------------------------------
// GetRoomMemberIDs
// --------------------------------------------------------------------------

func TestGetRoomMemberIDs_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"members": []string{"@alice:hs", "@bob:hs"},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	userIDs, err := sa.GetRoomMemberIDs(context.Background(), "!room1:hs")
	if err != nil {
		t.Fatalf("GetRoomMemberIDs: %v", err)
	}
	if len(userIDs) != 2 {
		t.Fatalf("expected 2 user IDs, got %d", len(userIDs))
	}
	if userIDs[0] != id.UserID("@alice:hs") {
		t.Errorf("expected @alice:hs, got %s", userIDs[0])
	}
	if userIDs[1] != id.UserID("@bob:hs") {
		t.Errorf("expected @bob:hs, got %s", userIDs[1])
	}
}

func TestGetRoomMemberIDs_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_NOT_FOUND",
			"error":   "Room not found",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRoomMemberIDs(context.Background(), "!missing:hs")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// --------------------------------------------------------------------------
// GetRoomState
// --------------------------------------------------------------------------

func TestGetRoomState_AllEvents(t *testing.T) {
	stateEvt1 := map[string]interface{}{"type": "m.room.create", "content": map[string]interface{}{"creator": "@admin:hs"}}
	stateEvt2 := map[string]interface{}{"type": "m.room.name", "content": map[string]interface{}{"name": "Test"}}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// No type filter when eventType is empty
		if r.URL.Query().Get("type") != "" {
			t.Error("expected no type query param for all events")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"state": []interface{}{stateEvt1, stateEvt2},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	events, err := sa.GetRoomState(context.Background(), "!room1:hs", "")
	if err != nil {
		t.Fatalf("GetRoomState: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 state events, got %d", len(events))
	}
}

func TestGetRoomState_FilteredByType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("type") != "m.room.name" {
			t.Errorf("expected type=m.room.name, got %s", r.URL.Query().Get("type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"state": []interface{}{
				map[string]interface{}{"type": "m.room.name", "content": map[string]interface{}{"name": "Test"}},
			},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	events, err := sa.GetRoomState(context.Background(), "!room1:hs", "m.room.name")
	if err != nil {
		t.Fatalf("GetRoomState: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 state event, got %d", len(events))
	}
}

func TestGetRoomState_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_FORBIDDEN",
			"error":   "Access denied",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRoomState(context.Background(), "!room1:hs", "")
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// --------------------------------------------------------------------------
// GetStateEventContent
// --------------------------------------------------------------------------

func TestGetStateEventContent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"state": []interface{}{
				map[string]interface{}{
					"type":    "m.room.name",
					"content": map[string]interface{}{"name": "Test Room"},
				},
			},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	content, err := sa.GetStateEventContent(context.Background(), "!room1:hs", "m.room.name")
	if err != nil {
		t.Fatalf("GetStateEventContent: %v", err)
	}
	if content == nil {
		t.Fatal("expected non-nil content")
	}
	if content["name"] != "Test Room" {
		t.Errorf("expected name=Test Room, got %v", content["name"])
	}
}

func TestGetStateEventContent_NoEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"state": []interface{}{},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	content, err := sa.GetStateEventContent(context.Background(), "!room1:hs", "m.room.name")
	if err != nil {
		t.Fatalf("GetStateEventContent: %v", err)
	}
	if content != nil {
		t.Errorf("expected nil content for empty state, got %v", content)
	}
}

func TestGetStateEventContent_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_UNKNOWN",
			"error":   "Server error",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetStateEventContent(context.Background(), "!room1:hs", "m.room.name")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// --------------------------------------------------------------------------
// GetCustomState
// --------------------------------------------------------------------------

func TestGetCustomState_SpecificTypes(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		typeFilter := r.URL.Query().Get("type")
		if typeFilter == "io.alkemio.metadata" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"state": []interface{}{
					map[string]interface{}{
						"type":    "io.alkemio.metadata",
						"content": map[string]interface{}{"key": "value"},
					},
				},
			})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"state": []interface{}{},
			})
		}
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	result, err := sa.GetCustomState(context.Background(), "!room1:hs", []string{"io.alkemio.metadata"})
	if err != nil {
		t.Fatalf("GetCustomState: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 custom state entry, got %d", len(result))
	}
	if result["io.alkemio.metadata"]["key"] != "value" {
		t.Errorf("expected key=value, got %v", result["io.alkemio.metadata"])
	}
}

func TestGetCustomState_AllTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"state": []interface{}{
				map[string]interface{}{
					"type":    "m.room.create",
					"content": map[string]interface{}{"creator": "@admin:hs"},
				},
				map[string]interface{}{
					"type":    "io.alkemio.room.type",
					"content": map[string]interface{}{"room_type": "discussion"},
				},
				map[string]interface{}{
					"type":    "io.alkemio.context",
					"content": map[string]interface{}{"context_id": "abc"},
				},
			},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	result, err := sa.GetCustomState(context.Background(), "!room1:hs", nil)
	if err != nil {
		t.Fatalf("GetCustomState: %v", err)
	}
	// Should only include io.alkemio.* events, not m.room.create
	if len(result) != 2 {
		t.Fatalf("expected 2 custom state entries, got %d", len(result))
	}
	if _, ok := result["m.room.create"]; ok {
		t.Error("should not include m.room.create")
	}
	if result["io.alkemio.room.type"]["room_type"] != "discussion" {
		t.Errorf("expected room_type=discussion, got %v", result["io.alkemio.room.type"])
	}
	if result["io.alkemio.context"]["context_id"] != "abc" {
		t.Errorf("expected context_id=abc, got %v", result["io.alkemio.context"])
	}
}

func TestGetCustomState_InvalidPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":[]}`))
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetCustomState(context.Background(), "!room1:hs", []string{"m.room.name"})
	if err == nil {
		t.Fatal("expected error for non-io.alkemio prefix")
	}
	if !strings.Contains(err.Error(), "io.alkemio.") {
		t.Errorf("error should mention io.alkemio. prefix, got: %v", err)
	}
}

func TestGetCustomState_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_FORBIDDEN",
			"error":   "Forbidden",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetCustomState(context.Background(), "!room1:hs", []string{"io.alkemio.metadata"})
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// --------------------------------------------------------------------------
// GetRoomMessages
// --------------------------------------------------------------------------

func TestGetRoomMessages_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/messages") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("dir") != "b" {
			t.Errorf("expected dir=b, got %s", q.Get("dir"))
		}
		if q.Get("limit") != "50" {
			t.Errorf("expected limit=50, got %s", q.Get("limit"))
		}
		if q.Get("from") != "t123" {
			t.Errorf("expected from=t123, got %s", q.Get("from"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"start": "t123",
			"end":   "t456",
			"chunk": []map[string]interface{}{
				{
					"type":             "m.room.message",
					"event_id":         "$msg1:hs",
					"sender":           "@alice:hs",
					"origin_server_ts": 1000,
					"content":          map[string]interface{}{"msgtype": "m.text", "body": "hello"},
				},
			},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	resp, err := sa.GetRoomMessages(context.Background(), "!room1:hs", "t123", "b", 50)
	if err != nil {
		t.Fatalf("GetRoomMessages: %v", err)
	}
	if resp.Start != "t123" {
		t.Errorf("expected start=t123, got %s", resp.Start)
	}
	if resp.End != "t456" {
		t.Errorf("expected end=t456, got %s", resp.End)
	}
	if len(resp.Chunk) != 1 {
		t.Fatalf("expected 1 message, got %d", len(resp.Chunk))
	}
	if resp.Chunk[0].ID != "$msg1:hs" {
		t.Errorf("expected event_id=$msg1:hs, got %s", resp.Chunk[0].ID)
	}
}

func TestGetRoomMessages_NoFrom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("from") != "" {
			t.Errorf("expected no from param, got %s", r.URL.Query().Get("from"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"start": "",
			"chunk": []interface{}{},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRoomMessages(context.Background(), "!room1:hs", "", "f", 10)
	if err != nil {
		t.Fatalf("GetRoomMessages: %v", err)
	}
}

func TestGetRoomMessages_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_NOT_FOUND",
			"error":   "Room not found",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRoomMessages(context.Background(), "!missing:hs", "", "b", 10)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// --------------------------------------------------------------------------
// GetEventContext
// --------------------------------------------------------------------------

func TestGetEventContext_Success(t *testing.T) {
	sa, err := NewSynapseAdmin("http://homeserver.test", "test-token")
	if err != nil {
		t.Fatalf("NewSynapseAdmin: %v", err)
	}
	sa.client.Client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/context/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "0" {
			t.Errorf("expected context limit=0, got %q", got)
		}
		body, marshalErr := json.Marshal(map[string]interface{}{
			"event": map[string]interface{}{
				"type":             "m.room.message",
				"event_id":         "$evt1:hs",
				"sender":           "@alice:hs",
				"origin_server_ts": 1000,
				"content":          map[string]interface{}{"msgtype": "m.text", "body": "test"},
			},
			"start":         "s1",
			"end":           "s2",
			"events_before": []interface{}{},
			"events_after":  []interface{}{},
			"state":         []interface{}{},
		})
		if marshalErr != nil {
			return nil, marshalErr
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(string(body))),
		}, nil
	})}

	resp, err := sa.GetEventContext(context.Background(), "!room1:hs", "$evt1:hs")
	if err != nil {
		t.Fatalf("GetEventContext: %v", err)
	}
	if resp.Event == nil {
		t.Fatal("expected non-nil event")
	}
	if resp.Event.ID != "$evt1:hs" {
		t.Errorf("expected event_id=$evt1:hs, got %s", resp.Event.ID)
	}
}

func TestGetEventContext_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_NOT_FOUND",
			"error":   "Event not found",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetEventContext(context.Background(), "!room1:hs", "$missing:hs")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// --------------------------------------------------------------------------
// GetEvent (wrapper around GetEventContext)
// --------------------------------------------------------------------------

func TestGetEvent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"event": map[string]interface{}{
				"type":             "m.room.message",
				"event_id":         "$evt1:hs",
				"sender":           "@alice:hs",
				"origin_server_ts": 2000,
				"content":          map[string]interface{}{"msgtype": "m.text", "body": "hello world"},
			},
			"start":         "s1",
			"end":           "s2",
			"events_before": []interface{}{},
			"events_after":  []interface{}{},
			"state":         []interface{}{},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	evt, err := sa.GetEvent(context.Background(), "!room1:hs", "$evt1:hs")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if evt == nil {
		t.Fatal("expected non-nil event")
	}
	if evt.ID != "$evt1:hs" {
		t.Errorf("expected event_id=$evt1:hs, got %s", evt.ID)
	}
	if evt.Sender != "@alice:hs" {
		t.Errorf("expected sender=@alice:hs, got %s", evt.Sender)
	}
}

func TestGetEvent_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_NOT_FOUND",
			"error":   "Event not found",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetEvent(context.Background(), "!room1:hs", "$missing:hs")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// --------------------------------------------------------------------------
// GetTimestampToEvent
// --------------------------------------------------------------------------

func TestGetTimestampToEvent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		q := r.URL.Query()
		if q.Get("ts") != "1609459200000" {
			t.Errorf("expected ts=1609459200000, got %s", q.Get("ts"))
		}
		if q.Get("dir") != "f" {
			t.Errorf("expected dir=f, got %s", q.Get("dir"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"event_id": "$closest:hs",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	eventID, err := sa.GetTimestampToEvent(context.Background(), "!room1:hs", 1609459200000, "f")
	if err != nil {
		t.Fatalf("GetTimestampToEvent: %v", err)
	}
	if eventID != "$closest:hs" {
		t.Errorf("expected event_id=$closest:hs, got %s", eventID)
	}
}

func TestGetTimestampToEvent_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_NOT_FOUND",
			"error":   "No event found",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetTimestampToEvent(context.Background(), "!room1:hs", 1609459200000, "f")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

// --------------------------------------------------------------------------
// GetRelations
// --------------------------------------------------------------------------

func TestGetRelations_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// Uses client API path: /_matrix/client/v1/rooms/.../relations/...
		if !strings.Contains(r.URL.Path, "/_matrix/client/") {
			t.Errorf("expected client API path, got %s", r.URL.Path)
		}
		if !strings.Contains(r.URL.Path, "/relations/") {
			t.Errorf("expected /relations/ in path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"chunk": []map[string]interface{}{
				{
					"type":             "m.reaction",
					"event_id":         "$reaction1:hs",
					"sender":           "@bob:hs",
					"origin_server_ts": 3000,
					"content": map[string]interface{}{
						"m.relates_to": map[string]interface{}{
							"rel_type": "m.annotation",
							"event_id": "$evt1:hs",
							"key":      "👍",
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	events, err := sa.GetRelations(
		context.Background(), "!room1:hs", "$evt1:hs",
		event.RelAnnotation, event.EventReaction,
	)
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(events))
	}
	if events[0].ID != "$reaction1:hs" {
		t.Errorf("expected event_id=$reaction1:hs, got %s", events[0].ID)
	}
}

// GetRelations follows next_batch and accumulates every page. Simulates a thread
// whose first page is all sticker replies (newest-first) and whose second page
// carries an older message reply: without pagination the message reply would be
// dropped. Also asserts the empty event-type filter omits the type path segment.
func TestGetRelations_Paginates(t *testing.T) {
	var gotFroms []string
	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFroms = append(gotFroms, r.URL.Query().Get("from"))
		gotPaths = append(gotPaths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("from") == "" {
			// Page 1: sticker replies + next_batch pointing at page 2.
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"chunk": []map[string]interface{}{
					{"type": "m.sticker", "event_id": "$s1:hs", "sender": "@a:hs", "origin_server_ts": 3000},
					{"type": "m.sticker", "event_id": "$s2:hs", "sender": "@a:hs", "origin_server_ts": 2900},
				},
				"next_batch": "PAGE2",
			})
			return
		}
		// Page 2: an older message reply, no further next_batch.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"chunk": []map[string]interface{}{
				{"type": "m.room.message", "event_id": "$m1:hs", "sender": "@b:hs", "origin_server_ts": 1000},
			},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	events, err := sa.GetRelations(
		context.Background(), "!room1:hs", "$root:hs",
		event.RelThread, event.Type{}, // empty type → all relation event types
	)
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 accumulated relations across 2 pages, got %d", len(events))
	}
	if events[0].ID != "$s1:hs" || events[2].ID != "$m1:hs" {
		t.Errorf("expected [stickers..., message], got %s .. %s", events[0].ID, events[2].ID)
	}
	if len(gotFroms) != 2 || gotFroms[0] != "" || gotFroms[1] != "PAGE2" {
		t.Errorf("expected 2 requests with from=[\"\", \"PAGE2\"], got %v", gotFroms)
	}
	// Empty event-type must NOT add a trailing type segment after the relType.
	if strings.HasSuffix(gotPaths[0], "/m.thread/") || strings.Contains(gotPaths[0], "/m.thread/m.") {
		t.Errorf("expected no event-type path segment for empty type, got %s", gotPaths[0])
	}
}

// A later-page failure returns the pages accumulated so far (best-effort), not an
// error: page 1 succeeds with a next_batch, page 2 returns 500 → GetRelations
// yields page 1's relations with no error, so a transient blip can't collapse a
// multi-page thread to nothing.
func TestGetRelations_LaterPageError_ReturnsPartial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("from") == "" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"chunk": []map[string]interface{}{
					{"type": "m.room.message", "event_id": "$m1:hs", "sender": "@a:hs", "origin_server_ts": 3000},
				},
				"next_batch": "PAGE2",
			})
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"errcode": "M_UNKNOWN"})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	events, err := sa.GetRelations(
		context.Background(), "!room1:hs", "$root:hs",
		event.RelThread, event.Type{},
	)
	if err != nil {
		t.Fatalf("expected no error on later-page failure (partial results), got %v", err)
	}
	if len(events) != 1 || events[0].ID != "$m1:hs" {
		t.Fatalf("expected page 1's relation returned, got %d events", len(events))
	}
}

// A FIRST-page failure is a genuine error (nothing to show).
func TestGetRelations_FirstPageError_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"errcode": "M_UNKNOWN"})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRelations(
		context.Background(), "!room1:hs", "$root:hs",
		event.RelThread, event.Type{},
	)
	if err == nil {
		t.Fatal("expected an error when the first page fails")
	}
}

func TestGetRelations_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"chunk": []interface{}{},
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	events, err := sa.GetRelations(
		context.Background(), "!room1:hs", "$evt1:hs",
		event.RelAnnotation, event.EventReaction,
	)
	if err != nil {
		t.Fatalf("GetRelations: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 relations, got %d", len(events))
	}
}

func TestGetRelations_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_FORBIDDEN",
			"error":   "Not in room",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRelations(
		context.Background(), "!room1:hs", "$evt1:hs",
		event.RelAnnotation, event.EventReaction,
	)
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// --------------------------------------------------------------------------
// JoinRoom
// --------------------------------------------------------------------------

func TestJoinRoom_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/_synapse/admin/v1/join/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.JoinRoom(context.Background(), "!room1:hs", "@alice:hs")
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if receivedBody["user_id"] != "@alice:hs" {
		t.Errorf("expected user_id=@alice:hs, got %v", receivedBody["user_id"])
	}
}

func TestJoinRoom_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_FORBIDDEN",
			"error":   "Not admin",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	err := sa.JoinRoom(context.Background(), "!room1:hs", "@alice:hs")
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

// --------------------------------------------------------------------------
// GetRegistrationNonce
// --------------------------------------------------------------------------

func TestGetRegistrationNonce_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/_synapse/admin/v1/register") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"nonce": "abc123nonce",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	nonce, err := sa.GetRegistrationNonce(context.Background())
	if err != nil {
		t.Fatalf("GetRegistrationNonce: %v", err)
	}
	if nonce != "abc123nonce" {
		t.Errorf("expected nonce=abc123nonce, got %s", nonce)
	}
}

func TestGetRegistrationNonce_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_UNKNOWN",
			"error":   "Server error",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.GetRegistrationNonce(context.Background())
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

// --------------------------------------------------------------------------
// RegisterWithMAC
// --------------------------------------------------------------------------

func TestRegisterWithMAC_Success(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/_synapse/admin/v1/register") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "new-access-token-123",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	token, err := sa.RegisterWithMAC(context.Background(), "nonce1", "newuser", "password123", "hmac-value", true)
	if err != nil {
		t.Fatalf("RegisterWithMAC: %v", err)
	}
	if token != "new-access-token-123" {
		t.Errorf("expected access_token=new-access-token-123, got %s", token)
	}
	if receivedBody["nonce"] != "nonce1" {
		t.Errorf("expected nonce=nonce1, got %v", receivedBody["nonce"])
	}
	if receivedBody["username"] != "newuser" {
		t.Errorf("expected username=newuser, got %v", receivedBody["username"])
	}
	if receivedBody["password"] != "password123" {
		t.Errorf("expected password=password123, got %v", receivedBody["password"])
	}
	if receivedBody["mac"] != "hmac-value" {
		t.Errorf("expected mac=hmac-value, got %v", receivedBody["mac"])
	}
	if admin, ok := receivedBody["admin"].(bool); !ok || !admin {
		t.Errorf("expected admin=true, got %v", receivedBody["admin"])
	}
}

func TestRegisterWithMAC_NonAdmin(t *testing.T) {
	var receivedBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token-456",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	token, err := sa.RegisterWithMAC(context.Background(), "nonce2", "user2", "pass", "mac2", false)
	if err != nil {
		t.Fatalf("RegisterWithMAC: %v", err)
	}
	if token != "token-456" {
		t.Errorf("expected token-456, got %s", token)
	}
	if admin, ok := receivedBody["admin"].(bool); !ok || admin {
		t.Errorf("expected admin=false, got %v", receivedBody["admin"])
	}
}

func TestRegisterWithMAC_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"errcode": "M_UNKNOWN",
			"error":   "Invalid MAC",
		})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, err := sa.RegisterWithMAC(context.Background(), "bad-nonce", "user", "pass", "bad-mac", false)
	if err == nil {
		t.Fatal("expected error for 400")
	}
}

// --------------------------------------------------------------------------
// Authorization header
// --------------------------------------------------------------------------

func TestAccessTokenSentInRequests(t *testing.T) {
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"admin": false})
	}))
	defer srv.Close()

	sa := newTestSynapseAdmin(t, srv)
	_, _ = sa.GetUser(context.Background(), "@test:hs")
	if authHeader != "Bearer test-token" {
		t.Errorf("expected Authorization: Bearer test-token, got %q", authHeader)
	}
}

// --------------------------------------------------------------------------
// Server down (connection refused)
// --------------------------------------------------------------------------

func TestServerDown_ReturnsError(t *testing.T) {
	// Create and immediately close the server to get an unreachable URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	sa, err := NewSynapseAdmin(url, "tok")
	if err != nil {
		t.Fatalf("NewSynapseAdmin: %v", err)
	}
	_, err = sa.GetUser(context.Background(), "@alice:hs")
	if err == nil {
		t.Fatal("expected error when server is down")
	}
}

package matrix

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"maunium.net/go/mautrix/event"
)

func TestAlkemioVisibilityTypeMapRegistration(t *testing.T) {
	registered, ok := event.TypeMap[StateAlkemioVisibility]
	if !ok {
		t.Fatal("StateAlkemioVisibility not registered in event.TypeMap")
	}
	expected := reflect.TypeOf(AlkemioVisibilityContent{})
	if registered != expected {
		t.Errorf("TypeMap maps to %v, want %v", registered, expected)
	}
}

func TestAlkemioEntityRoundTrip(t *testing.T) {
	registered, ok := event.TypeMap[StateAlkemioEntity]
	if !ok {
		t.Fatal("StateAlkemioEntity not registered in event.TypeMap")
	}
	if expected := reflect.TypeOf(AlkemioEntityContent{}); registered != expected {
		t.Errorf("TypeMap maps to %v, want %v", registered, expected)
	}
	parent := "9a2f3c44-0000-4000-8000-000000000001"
	in := AlkemioEntityContent{EntityID: "11111111-2222-4333-8444-555555555555", EntityType: "thread", ParentID: &parent}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out AlkemioEntityContent
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.EntityID != in.EntityID || out.EntityType != in.EntityType || out.ParentID == nil || *out.ParentID != parent {
		t.Errorf("round trip mismatch: %+v != %+v", out, in)
	}
	if !strings.Contains(string(raw), `"entityId"`) || !strings.Contains(string(raw), `"parentId"`) {
		t.Errorf("JSON field names drifted: %s", raw)
	}
}

func TestAlkemioGovernanceRoundTrip(t *testing.T) {
	registered, ok := event.TypeMap[StateAlkemioGovernance]
	if !ok {
		t.Fatal("StateAlkemioGovernance not registered in event.TypeMap")
	}
	if expected := reflect.TypeOf(AlkemioGovernanceContent{}); registered != expected {
		t.Errorf("TypeMap maps to %v, want %v", registered, expected)
	}
	in := AlkemioGovernanceContent{LadderVersion: 1, MembershipMode: "platform", AppliedAt: 1757000000000, RoomVersion: "10"}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out AlkemioGovernanceContent
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != in {
		t.Errorf("round trip mismatch: %+v != %+v", out, in)
	}
	if !strings.Contains(string(raw), `"ladderVersion"`) || !strings.Contains(string(raw), `"membershipMode"`) {
		t.Errorf("JSON field names drifted: %s", raw)
	}
}

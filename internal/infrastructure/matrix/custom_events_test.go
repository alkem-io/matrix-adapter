package matrix

import (
	"reflect"
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

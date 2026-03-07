package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIDMapper_UserID(t *testing.T) {
	mapper := NewIDMapper("example.com")

	actorID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	userID := mapper.UserID(actorID)

	assert.Equal(t, "@12345678-1234-1234-1234-123456789abc:example.com", userID.String())
}

func TestIDMapper_AlkemioActorID(t *testing.T) {
	mapper := NewIDMapper("example.com")

	actorID := mapper.AlkemioActorID("@12345678-1234-1234-1234-123456789abc:example.com")

	assert.Equal(t, uuid.MustParse("12345678-1234-1234-1234-123456789abc"), actorID)
}

func TestIDMapper_AlkemioActorID_InvalidUUID(t *testing.T) {
	mapper := NewIDMapper("example.com")

	actorID := mapper.AlkemioActorID("@not-a-uuid:example.com")

	assert.Equal(t, uuid.Nil, actorID)
}

func TestIDMapper_RoomAlias(t *testing.T) {
	mapper := NewIDMapper("example.com")

	roomID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	alias := mapper.RoomAlias(roomID)

	assert.Equal(t, "#aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:example.com", alias)
}

func TestIDMapper_AlkemioRoomID(t *testing.T) {
	mapper := NewIDMapper("example.com")

	roomUUID := mapper.AlkemioRoomID("#aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:example.com")

	assert.Equal(t, uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"), roomUUID)
}

func TestIDMapper_AlkemioRoomID_WrongDomain(t *testing.T) {
	mapper := NewIDMapper("example.com")

	roomUUID := mapper.AlkemioRoomID("#aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee:other.com")

	assert.Equal(t, uuid.Nil, roomUUID)
}

func TestIDMapper_SpaceAlias(t *testing.T) {
	mapper := NewIDMapper("example.com")

	contextID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	alias := mapper.SpaceAlias(contextID)

	assert.Equal(t, "#11111111-2222-3333-4444-555555555555:example.com", alias)
}

func TestIDMapper_HomeserverDomain(t *testing.T) {
	mapper := NewIDMapper("matrix.example.org")

	assert.Equal(t, "matrix.example.org", mapper.HomeserverDomain())
}

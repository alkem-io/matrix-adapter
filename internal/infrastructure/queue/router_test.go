package queue

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// The send partition key is the target room, so same-room sends serialize while
// different rooms run concurrently.
func TestSendMessagePartitionKey_ExtractsRoom(t *testing.T) {
	roomID := dto.AlkemioRoomID(uuid.MustParse("11111111-1111-4111-8111-111111111111"))
	payload, err := json.Marshal(dto.SendMessageRequest{
		AlkemioRoomID: roomID,
		SenderActorID: dto.AlkemioActorID(uuid.New()),
		Content:       "hi",
	})
	assert.NoError(t, err)
	assert.Equal(t, roomID.String(), sendMessagePartitionKey(payload))
}

// Two sends to the same room map to the same key (ordering), two rooms differ.
func TestSendMessagePartitionKey_StableAndDistinct(t *testing.T) {
	roomA := dto.AlkemioRoomID(uuid.New())
	roomB := dto.AlkemioRoomID(uuid.New())
	mk := func(r dto.AlkemioRoomID) []byte {
		b, _ := json.Marshal(dto.SendMessageRequest{AlkemioRoomID: r})
		return b
	}
	assert.Equal(t, sendMessagePartitionKey(mk(roomA)), sendMessagePartitionKey(mk(roomA)))
	assert.NotEqual(t, sendMessagePartitionKey(mk(roomA)), sendMessagePartitionKey(mk(roomB)))
}

// A malformed payload falls back to the empty key rather than panicking or
// dropping the message.
func TestSendMessagePartitionKey_MalformedFallsBack(t *testing.T) {
	assert.Equal(t, "", sendMessagePartitionKey([]byte("not json")))
}

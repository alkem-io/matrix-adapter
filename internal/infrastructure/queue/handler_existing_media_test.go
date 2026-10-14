package queue

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHandleSendMessage_ExistingMediaWireAlternative(t *testing.T) {
	for _, reply := range []bool{false, true} {
		t.Run(map[bool]string{false: "message", true: "reply"}[reply], func(t *testing.T) {
			mock := &testMockMatrixPort{resolveAliasResult: "!room1:test", sendMessageResult: "$sent", sendReplyResult: "$reply"}
			h := testRoomHandler(mock)
			payload := map[string]any{
				"alkemio_room_id": uuid.NewString(), "sender_actor_id": uuid.NewString(), "content": "",
				"existing_media": map[string]any{"media_id": "alreadyUploaded", "display_name": "résumé.png", "mime_type": "image/png", "size": 42},
			}
			if reply {
				payload["parent_message_id"] = "$parent"
			}
			result, err := h.HandleSendMessage(context.Background(), mustMarshal(t, payload))
			require.NoError(t, err)
			assertSuccess(t, result)
			attachments := mock.capturedSendMessageAttachments
			if reply {
				attachments = mock.capturedSendReplyAttachments
			}
			require.Len(t, attachments, 1)
			require.Equal(t, "alreadyUploaded", attachments[0].MediaID)
			require.Equal(t, "résumé.png", attachments[0].DisplayName)
		})
	}
}

func TestHandleSendMessage_ExistingMediaRejectsCompetingAlternatives(t *testing.T) {
	for _, extra := range []map[string]any{
		{"content": "not attachment-only"},
	} {
		mock := &testMockMatrixPort{resolveAliasResult: "!room1:test", sendMessageResult: "$sent"}
		h := testRoomHandler(mock)
		payload := map[string]any{
			"alkemio_room_id": uuid.NewString(), "sender_actor_id": uuid.NewString(), "content": "",
			"existing_media": map[string]any{"media_id": "alreadyUploaded", "display_name": "file", "mime_type": "image/png", "size": 42},
		}
		for key, value := range extra {
			payload[key] = value
		}
		result, err := h.HandleSendMessage(context.Background(), mustMarshal(t, payload))
		require.NoError(t, err)
		assertErrorMessageContains(t, result, "either text or one attachment")
		require.Empty(t, mock.capturedSendMessageAttachments)
		require.Empty(t, mock.capturedSendMessageContent)
	}
}

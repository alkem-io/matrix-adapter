package queue

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alkem-io/matrix-adapter/pkg/dto"
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

func TestValidateSendAlternatives(t *testing.T) {
	media := &dto.ExistingMediaRef{MediaID: "alreadyUploaded", DisplayName: "photo.png", MimeType: "image/png", Size: 42}
	for _, tc := range []struct {
		name    string
		content string
		media   *dto.ExistingMediaRef
		wantErr bool
	}{
		{name: "text only", content: "hello"},
		{name: "media only", media: media},
		{name: "text and media", content: "look at this", media: media},
		{name: "neither", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := (&RoomHandler{}).validateSendAlternatives(dto.SendMessageRequest{Content: tc.content, ExistingMedia: tc.media})
			if !tc.wantErr {
				require.Nil(t, resp)
				return
			}
			require.NotNil(t, resp)
			assertErrorMessageContains(t, *resp, "send requires text, one attachment, or both")
		})
	}
}

// A caption and its media reach the port together, on both the message and the
// reply path, so the adapter publishes them as one event.
func TestHandleSendMessage_ExistingMediaWithCaption(t *testing.T) {
	for _, reply := range []bool{false, true} {
		t.Run(map[bool]string{false: "message", true: "reply"}[reply], func(t *testing.T) {
			mock := &testMockMatrixPort{resolveAliasResult: "!room1:test", sendMessageResult: "$sent", sendReplyResult: "$reply"}
			h := testRoomHandler(mock)
			payload := map[string]any{
				"alkemio_room_id": uuid.NewString(), "sender_actor_id": uuid.NewString(), "content": "look at this",
				"existing_media": map[string]any{"media_id": "alreadyUploaded", "display_name": "photo.png", "mime_type": "image/png", "size": 42},
			}
			if reply {
				payload["parent_message_id"] = "$parent"
			}
			result, err := h.HandleSendMessage(context.Background(), mustMarshal(t, payload))
			require.NoError(t, err)
			assertSuccess(t, result)
			content, attachments := mock.capturedSendMessageContent, mock.capturedSendMessageAttachments
			if reply {
				content, attachments = mock.capturedSendReplyContent, mock.capturedSendReplyAttachments
				require.Equal(t, "$parent", string(mock.capturedSendReplyThread))
			}
			require.Equal(t, "look at this", content)
			require.Len(t, attachments, 1)
			require.Equal(t, "photo.png", attachments[0].DisplayName)
			resp, ok := result.(dto.SendMessageResponse)
			require.True(t, ok)
			require.Equal(t, "look at this", resp.Content)
		})
	}
}

func TestHandleSendMessage_RejectsEmptySend(t *testing.T) {
	mock := &testMockMatrixPort{resolveAliasResult: "!room1:test", sendMessageResult: "$sent"}
	h := testRoomHandler(mock)
	payload := map[string]any{"alkemio_room_id": uuid.NewString(), "sender_actor_id": uuid.NewString(), "content": ""}
	result, err := h.HandleSendMessage(context.Background(), mustMarshal(t, payload))
	require.NoError(t, err)
	assertErrorMessageContains(t, result, "send requires text, one attachment, or both")
	require.Empty(t, mock.capturedSendMessageAttachments)
	require.Empty(t, mock.capturedSendMessageContent)
}

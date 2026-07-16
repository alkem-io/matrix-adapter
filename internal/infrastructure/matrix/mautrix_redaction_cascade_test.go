package matrix

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

const cascadeTestRoomID = id.RoomID("!room:test.local")

func configureCascadeScan(admin *mockAdminAPI, events ...*event.Event) {
	admin.getEventContextResult = &mautrix.RespContext{End: "after-primary"}
	admin.getRoomMessagesResult = &mautrix.RespMessages{Chunk: events}
}

func cascadeTestEvent(eventID id.EventID, content map[string]any) *event.Event {
	return &event.Event{
		ID:      eventID,
		Type:    event.EventMessage,
		Content: event.Content{Raw: content},
	}
}

func newCascadeMediaAdapter(
	t *testing.T, intent *mockIntentAPI, admin *mockAdminAPI,
) *MautrixAdapter {
	t.Helper()
	fileServiceURL := stubFileService(t, func(_ *http.Request) (*http.Response, error) {
		return fileServiceResponse(http.StatusOK, "image/png", []byte("PNG")), nil
	})
	a := newMediaTestAdapter(t, fileServiceURL, intent)
	a.admin = admin
	return a
}

// A text + media send stamps every attachment with the text event as parent,
// then deleting that primary redacts every stamped attachment.
func TestRedactEvent_CascadesNonThreadedAttachments(t *testing.T) {
	intent := &mockIntentAPI{
		sendTextResult:         &mautrix.RespSendEvent{EventID: "$primary"},
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/media")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$sent-media"},
	}
	admin := &mockAdminAPI{}
	a := newCascadeMediaAdapter(t, intent, admin)

	primaryEventID, err := a.SendMessage(
		context.Background(), cascadeTestRoomID, testActor(testActorID, "Alice"), "files",
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "one.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "two.png", MimeType: "image/png"},
		},
	)
	require.NoError(t, err)
	require.Equal(t, id.EventID("$primary"), primaryEventID)
	require.Len(t, intent.sendMsgEventContents, 2)

	firstContent, ok := intent.sendMsgEventContents[0].(map[string]any)
	require.True(t, ok)
	secondContent, ok := intent.sendMsgEventContents[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "$primary", firstContent[attachmentParentEventIDField])
	assert.Equal(t, "$primary", secondContent[attachmentParentEventIDField])
	assert.NotContains(t, firstContent, "m.relates_to")
	assert.NotContains(t, secondContent, "m.relates_to")

	configureCascadeScan(admin,
		cascadeTestEvent("$media-1", firstContent),
		cascadeTestEvent("$media-2", secondContent),
	)
	require.NoError(t, a.RedactEvent(
		context.Background(), cascadeTestRoomID, testActor(testActorID, "Alice"), primaryEventID, "deleted",
	))

	assert.Equal(t, []id.EventID{"$primary", "$media-1", "$media-2"}, intent.redactEventIDs)
	require.Len(t, admin.getRoomMessagesCalls, 1)
	assert.Equal(t, getRoomMessagesCall{
		RoomID: cascadeTestRoomID,
		From:   "after-primary",
		Dir:    "f",
		Limit:  attachmentCascadeScanLimit,
	}, admin.getRoomMessagesCalls[0])
}

// The custom parent marker coexists with the media event's one m.thread
// relation, allowing the same cascade mechanism to work for threaded sends.
func TestRedactEvent_CascadesThreadedAttachments(t *testing.T) {
	intent := &mockIntentAPI{
		uploadBytesResult:      &mautrix.RespMediaUpload{ContentURI: id.MustParseContentURI("mxc://test.local/media")},
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$primary"},
	}
	admin := &mockAdminAPI{}
	a := newCascadeMediaAdapter(t, intent, admin)

	const threadRootID = id.EventID("$thread-root")
	primaryEventID, err := a.SendReply(
		context.Background(), cascadeTestRoomID, testActor(testActorID, "Alice"), "files", threadRootID,
		[]domain.Attachment{
			{DocumentID: docID1, DisplayName: "one.png", MimeType: "image/png"},
			{DocumentID: docID2, DisplayName: "two.png", MimeType: "image/png"},
		},
	)
	require.NoError(t, err)
	require.Equal(t, id.EventID("$primary"), primaryEventID)
	require.Len(t, intent.sendMsgEventContents, 3, "thread text followed by two media events")

	attachmentContents := make([]map[string]any, 0, 2)
	for _, rawContent := range intent.sendMsgEventContents[1:] {
		content, ok := rawContent.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "$primary", content[attachmentParentEventIDField])
		relatesTo, ok := content["m.relates_to"].(*event.RelatesTo)
		require.True(t, ok)
		assert.Equal(t, event.RelThread, relatesTo.Type)
		assert.Equal(t, threadRootID, relatesTo.EventID)
		attachmentContents = append(attachmentContents, content)
	}

	configureCascadeScan(admin,
		cascadeTestEvent("$media-1", attachmentContents[0]),
		cascadeTestEvent("$media-2", attachmentContents[1]),
	)
	require.NoError(t, a.RedactEvent(
		context.Background(), cascadeTestRoomID, testActor(testActorID, "Alice"), primaryEventID, "deleted",
	))

	assert.Equal(t, []id.EventID{"$primary", "$media-1", "$media-2"}, intent.redactEventIDs)
}

func TestRedactEvent_PlainTextRedactsExactlyOneEvent(t *testing.T) {
	intent := &mockIntentAPI{}
	admin := &mockAdminAPI{}
	configureCascadeScan(admin)
	a := newFullTestAdapter(newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	}), admin)

	require.NoError(t, a.RedactEvent(
		context.Background(), cascadeTestRoomID, testActor(testActorID, "Alice"), "$plain", "deleted",
	))

	assert.Equal(t, []id.EventID{"$plain"}, intent.redactEventIDs)
	require.Len(t, admin.getEventContextCalls, 1)
	require.Len(t, admin.getRoomMessagesCalls, 1)
}

func TestRedactEvent_DoesNotRedactInterleavedForeignEvent(t *testing.T) {
	intent := &mockIntentAPI{}
	admin := &mockAdminAPI{}
	configureCascadeScan(admin,
		cascadeTestEvent("$media-1", map[string]any{
			attachmentParentEventIDField: "$primary",
		}),
		cascadeTestEvent("$foreign", map[string]any{
			"msgtype": "m.text",
			"body":    "unrelated",
		}),
		cascadeTestEvent("$other-message-attachment", map[string]any{
			attachmentParentEventIDField: "$other-primary",
		}),
		cascadeTestEvent("$media-2", map[string]any{
			attachmentParentEventIDField: "$primary",
		}),
	)
	a := newFullTestAdapter(newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	}), admin)

	require.NoError(t, a.RedactEvent(
		context.Background(), cascadeTestRoomID, testActor(testActorID, "Alice"), "$primary", "deleted",
	))

	assert.Equal(t, []id.EventID{"$primary", "$media-1", "$media-2"}, intent.redactEventIDs)
	assert.NotContains(t, intent.redactEventIDs, id.EventID("$foreign"))
	assert.NotContains(t, intent.redactEventIDs, id.EventID("$other-message-attachment"))
}

func TestRedactEvent_AttachmentFailureIsBestEffort(t *testing.T) {
	intent := &mockIntentAPI{
		redactEventErrs: map[id.EventID]error{
			"$media-1": assert.AnError,
		},
	}
	admin := &mockAdminAPI{}
	configureCascadeScan(admin,
		cascadeTestEvent("$media-1", map[string]any{attachmentParentEventIDField: "$primary"}),
		cascadeTestEvent("$media-2", map[string]any{attachmentParentEventIDField: "$primary"}),
	)
	a := newFullTestAdapter(newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	}), admin)

	require.NoError(t, a.RedactEvent(
		context.Background(), cascadeTestRoomID, testActor(testActorID, "Alice"), "$primary", "same reason",
	))

	assert.Equal(t, []id.EventID{"$primary", "$media-1", "$media-2"}, intent.redactEventIDs)
	assert.Equal(t, []string{"same reason", "same reason", "same reason"}, intent.redactReasons)
}

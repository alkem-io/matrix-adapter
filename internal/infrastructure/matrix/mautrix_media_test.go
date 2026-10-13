package matrix

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

const docID1 = "11111111-1111-4111-8111-111111111111"

type roundTripFunc func(*http.Request) (*http.Response, error)

func TestSendMessage_RejectsAggregateBeforePublishing(t *testing.T) {
	for _, content := range []string{"", "text"} {
		intent := &mockIntentAPI{}
		a := newMediaTestAdapter(t, intent)
		attachments := []domain.Attachment{{MediaID: "media1"}, {MediaID: "media2"}}
		if content != "" {
			attachments = attachments[:1]
		}
		message, err := a.SendMessage(context.Background(), "!room:test.local", testActor(testActorID, "Alice"), content, attachments)
		require.ErrorContains(t, err, "either text or one attachment")
		require.Nil(t, message)
		assert.Zero(t, intent.sendMessageEventCalled)
		assert.Zero(t, intent.uploadBytesCalled)
	}
}

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMediaTestAdapter(t *testing.T, intent *mockIntentAPI) *MautrixAdapter {
	t.Helper()
	return newMediaTestAdapterWithIntent(t, intent)
}

func newMediaTestAdapterWithIntent(t *testing.T, intent intentAPI) *MautrixAdapter {
	t.Helper()
	as := newMockAS(intent, map[id.UserID]intentAPI{
		expectedUserID(testActorID): intent,
	})
	a := newFullTestAdapter(as, &mockAdminAPI{})
	cfg := &config.Config{}
	a.cfg = cfg
	return a
}

func TestSendReply_ExistingMediaPreservesSenderThreadAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, filename, mime, messageType string
		size                              int64
		width, height                     *int
	}{
		{name: "image", filename: "résumé + photo.png", mime: "image/png", messageType: "m.image", size: 120, width: ptrInt(800), height: ptrInt(600)},
		{name: "video", filename: "clip.mp4", mime: "video/mp4", messageType: "m.video", size: 2048},
		{name: "audio", filename: "voice.ogg", mime: "audio/ogg", messageType: "m.audio", size: 456},
		{name: "empty file", filename: "empty.txt", mime: "text/plain; charset=utf-8", messageType: "m.file"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bot := &mockIntentAPI{}
			sender := &mockIntentAPI{sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$reply"}}
			as := newMockAS(bot, map[id.UserID]intentAPI{expectedUserID(testActorID): sender})
			a := newFullTestAdapter(as, &mockAdminAPI{})
			a.cfg = &config.Config{}
			attachment := domain.Attachment{
				MediaID: "already-uploaded", DisplayName: tc.filename,
				MimeType: tc.mime, Size: tc.size, Width: tc.width, Height: tc.height,
			}
			message, err := a.SendReply(context.Background(), "!reply-room:test.local",
				testActor(testActorID, "Alice"), "", "$thread-root", []domain.Attachment{attachment})
			require.NoError(t, err)
			require.NotNil(t, message)
			assert.Equal(t, 1, sender.sendMessageEventCalled, "the acting user's intent publishes the reply")
			assert.Zero(t, bot.sendMessageEventCalled, "the appservice bot does not publish for the user")
			assert.Zero(t, sender.uploadBytesCalled, "send must not create a second media ID")
			assert.Zero(t, bot.uploadBytesCalled)
			assert.Equal(t, id.RoomID("!reply-room:test.local"), sender.lastSendMsgEventRoomID)
			assert.Equal(t, event.EventMessage, sender.lastSendMsgEventType)
			content, ok := sender.lastSendMsgEventContent.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, tc.messageType, content["msgtype"])
			assert.Equal(t, tc.filename, content["body"])
			assert.Equal(t, "mxc://test.local/already-uploaded", content["url"])
			assert.NotContains(t, content, "io.alkemio.document_id")
			info, ok := content["info"].(map[string]any)
			require.True(t, ok)
			expectedMIME := tc.mime
			if tc.name == "empty file" {
				expectedMIME = "text/plain"
			}
			assert.Equal(t, expectedMIME, info["mimetype"])
			assert.Equal(t, tc.size, info["size"])
			if tc.width != nil {
				assert.Equal(t, *tc.width, info["w"])
				assert.Equal(t, *tc.height, info["h"])
			} else {
				assert.NotContains(t, info, "w")
				assert.NotContains(t, info, "h")
			}
			relationJSON, err := json.Marshal(content["m.relates_to"])
			require.NoError(t, err)
			assert.JSONEq(t, `{"rel_type":"m.thread","event_id":"$thread-root","m.in_reply_to":{"event_id":"$thread-root"},"is_falling_back":true}`, string(relationJSON))
			assert.Equal(t, testActorID, message.SenderID)
			assert.Equal(t, expectedUserID(testActorID).String(), message.SenderMatrixID)
			assert.Equal(t, "$thread-root", message.ThreadID)
			assert.Equal(t, "!reply-room:test.local", message.RoomID)
			assert.Equal(t, "$reply", message.ID)
			require.Len(t, message.Attachments, 1)
			assert.Equal(t, "already-uploaded", message.Attachments[0].MediaID)
			assert.Equal(t, expectedMIME, message.Attachments[0].MimeType)
			assert.Equal(t, tc.filename, message.Attachments[0].DisplayName)
		})
	}
}

func TestMediaMsgType_VideoAudio(t *testing.T) {
	assert.Equal(t, "m.video", mediaMsgType("video/mp4"))
	assert.Equal(t, "m.audio", mediaMsgType("audio/mpeg"))
	assert.Equal(t, "m.image", mediaMsgType("image/gif"))
	assert.Equal(t, "m.file", mediaMsgType("application/zip"))
	assert.Equal(t, "m.file", mediaMsgType(""))
}

func TestMaxAttachmentBytes_NonPositiveFallsBackToDefault(t *testing.T) {
	intent := &mockIntentAPI{}
	a := newMediaTestAdapter(t, intent)

	a.cfg.FileService.MaxAttachmentBytes = -1
	assert.Equal(t, config.DefaultMaxAttachmentBytes, a.cfg.MaxAttachmentBytes())

	a.cfg.FileService.MaxAttachmentBytes = 0
	assert.Equal(t, config.DefaultMaxAttachmentBytes, a.cfg.MaxAttachmentBytes())

	a.cfg.FileService.MaxAttachmentBytes = 1234
	assert.Equal(t, int64(1234), a.cfg.MaxAttachmentBytes())
}

func TestGetMessage_MediaEvent_ReturnsAttachment(t *testing.T) {
	ghost := expectedUserID(testActorID) // @<uuid>:test.local — an own-appservice ghost
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$m1"),
			Sender:    ghost,
			Type:      event.EventMessage,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"msgtype":                "m.image",
					"body":                   "photo.jpg",
					"url":                    "mxc://test.local/media123",
					"info":                   map[string]any{"mimetype": "image/jpeg", "size": float64(9)},
					"io.alkemio.document_id": docID1,
				},
			},
		},
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$m1")
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "photo.jpg", msg.Content, "the media body reaches Content verbatim")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "media123", msg.Attachments[0].MediaID)
	assert.Equal(t, "photo.jpg", msg.Attachments[0].DisplayName)
}

func TestGetMessage_Sticker_ReturnsAttachment(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$sticker1"),
			Sender:    id.UserID("@element:test.local"), // a normal Element user
			Type:      event.EventSticker,               // no msgtype field on stickers
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"body": "party parrot",
					"url":  "mxc://test.local/stickerid",
					"info": map[string]any{"mimetype": "image/png", "w": float64(128), "h": float64(128)},
				},
			},
		},
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$sticker1")
	require.NoError(t, err, "GetMessage on a sticker must not error (regression)")
	require.NotNil(t, msg)
	assert.Empty(t, msg.Content, "sticker alt-text must not be duplicated as Content")
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, "stickerid", msg.Attachments[0].MediaID, "MediaID is the re-home key")
	assert.Equal(t, "party parrot", msg.Attachments[0].DisplayName)
	assert.Equal(t, "image/png", msg.Attachments[0].MimeType)
}

func TestGetMessage_Sticker_MalformedBody_Errors(t *testing.T) {
	admin := &mockAdminAPI{
		getEventResult: &event.Event{
			ID:        id.EventID("$badsticker"),
			Sender:    id.UserID("@element:test.local"),
			Type:      event.EventSticker,
			Timestamp: 1700000000000,
			Content: event.Content{
				Raw: map[string]any{
					"body": map[string]any{"not": "a string"}, // present but non-string
					// no url → no attachment
				},
			},
		},
	}
	as := newMockAS(&mockIntentAPI{}, nil)
	a := newFullTestAdapter(as, admin)

	msg, err := a.GetMessage(context.Background(), "!room:test.local", "$badsticker")
	require.Error(t, err, "a malformed-body sticker with no media must error, not return a blank message")
	assert.Nil(t, msg)
}

func TestCountUnreadMessages_CountsStickers(t *testing.T) {
	other := id.UserID("@element:test.local")
	me := expectedUserID(testActorID)
	intent := &mockIntentAPI{
		messagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{Type: event.EventMessage, ID: "$t1", Sender: other, Content: event.Content{
					Raw: map[string]any{"msgtype": "m.text", "body": "hi"},
				}},
				{Type: event.EventSticker, ID: "$s1", Sender: other, Content: event.Content{
					Raw: map[string]any{"body": "party parrot", "url": "mxc://test.local/stickerid"},
				}},
			},
			End: "", // no more events → scan stops after this batch
		},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	count, ok := a.countUnreadMessages(context.Background(), intent, "!room:test.local", nil, me)
	require.True(t, ok, "scan completes (all events seen)")
	assert.Equal(t, 2, count, "both the text message and the sticker count as unread")
}

func TestCountUnreadMessages_SkipsOwnSticker(t *testing.T) {
	me := expectedUserID(testActorID)
	intent := &mockIntentAPI{
		messagesResult: &mautrix.RespMessages{
			Chunk: []*event.Event{
				{Type: event.EventSticker, ID: "$s1", Sender: me, Content: event.Content{
					Raw: map[string]any{"body": "wave", "url": "mxc://test.local/x"},
				}},
			},
			End: "",
		},
	}
	a := newFullTestAdapter(newMockAS(intent, nil), &mockAdminAPI{})

	count, ok := a.countUnreadMessages(context.Background(), intent, "!room:test.local", nil, me)
	require.True(t, ok)
	assert.Equal(t, 0, count, "a self-authored sticker is not unread")
}

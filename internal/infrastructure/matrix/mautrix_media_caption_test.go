package matrix

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

func captionTestAttachment() domain.Attachment {
	return domain.Attachment{
		MediaID: "already-uploaded", DisplayName: "sunset.png",
		MimeType: "image/png", Size: 120, Width: ptrInt(800), Height: ptrInt(600),
	}
}

func TestBuildMediaContent_Caption(t *testing.T) {
	mxc := id.ContentURI{Homeserver: "test.local", FileID: "already-uploaded"}

	t.Run("without caption body is the file name and filename is absent", func(t *testing.T) {
		content := buildMediaContent(captionTestAttachment(), "", mxc, "image/png", 120, "")
		assert.Equal(t, "sunset.png", content["body"])
		assert.NotContains(t, content, "filename")
		assert.Equal(t, "m.image", content["msgtype"])
		assert.Equal(t, "mxc://test.local/already-uploaded", content["url"])
		assert.NotContains(t, content, "m.relates_to")
	})

	t.Run("with caption body is the caption and filename is the file name", func(t *testing.T) {
		content := buildMediaContent(captionTestAttachment(), "look at this", mxc, "image/png", 120, "")
		assert.Equal(t, "look at this", content["body"])
		assert.Equal(t, "sunset.png", content["filename"])
		assert.Equal(t, "m.image", content["msgtype"])
		assert.Equal(t, "mxc://test.local/already-uploaded", content["url"])
		assert.NotContains(t, content, "m.relates_to")
	})
}

// sendCaptioned publishes one captioned media event through SendReply and returns
// the returned message plus the event content exactly as it would go on the
// wire (a JSON round trip, so the listener sees the same shape Synapse serves).
func sendCaptioned(t *testing.T, caption string, threadID id.EventID) (*domain.Message, map[string]any) {
	t.Helper()
	bot := &mockIntentAPI{}
	sender := &mockIntentAPI{sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$captioned"}}
	as := newMockAS(bot, map[id.UserID]intentAPI{expectedUserID(testActorID): sender})
	a := newFullTestAdapter(as, &mockAdminAPI{})
	a.cfg = &config.Config{}

	message, err := a.SendReply(context.Background(), "!room:test.local",
		testActor(testActorID, "Alice"), caption, threadID, []domain.Attachment{captionTestAttachment()})
	require.NoError(t, err)
	require.NotNil(t, message)
	assert.Equal(t, 1, sender.sendMessageEventCalled, "text and media publish as ONE event")
	assert.Zero(t, bot.sendMessageEventCalled)
	assert.Zero(t, sender.uploadBytesCalled, "send must not create a second media ID")

	wire, err := json.Marshal(sender.lastSendMsgEventContent)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(wire, &raw))
	return message, raw
}

func TestSendReply_CaptionedMediaRoundTripsThroughListener(t *testing.T) {
	for _, tc := range []struct {
		name, caption, wantContent string
		wantFilename               bool
	}{
		{name: "captioned", caption: "look at this", wantContent: "look at this", wantFilename: true},
		{name: "uncaptioned", caption: "", wantContent: "sunset.png"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message, raw := sendCaptioned(t, tc.caption, "")
			assert.Equal(t, tc.wantContent, raw["body"])
			if tc.wantFilename {
				assert.Equal(t, "sunset.png", raw["filename"])
			} else {
				assert.NotContains(t, raw, "filename")
			}

			// The send response carries the same Content the read path derives.
			assert.Equal(t, tc.wantContent, message.Content)
			require.Len(t, message.Attachments, 1)
			assert.Equal(t, "sunset.png", message.Attachments[0].DisplayName)

			evt := &event.Event{Type: event.EventMessage, Content: event.Content{Raw: raw}}
			content, attachment, ok := extractInboundMessage(evt, testIDMapper)
			require.True(t, ok)
			assert.Equal(t, tc.wantContent, content)
			require.NotNil(t, attachment)
			assert.Equal(t, "sunset.png", attachment.DisplayName)
			assert.Equal(t, "already-uploaded", attachment.MediaID)
			assert.Equal(t, "image/png", attachment.MimeType)
			assert.Equal(t, int64(120), attachment.Size)
			assert.Empty(t, extractThreadID(evt))
		})
	}
}

func TestSendReply_CaptionedMediaKeepsThreadRelation(t *testing.T) {
	message, raw := sendCaptioned(t, "look at this", "$thread-root")

	relationJSON, err := json.Marshal(raw["m.relates_to"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"rel_type":"m.thread","event_id":"$thread-root","m.in_reply_to":{"event_id":"$thread-root"},"is_falling_back":true}`, string(relationJSON))
	assert.Equal(t, "look at this", raw["body"])
	assert.Equal(t, "sunset.png", raw["filename"])
	assert.Equal(t, "$thread-root", message.ThreadID)
	assert.Equal(t, "look at this", message.Content)

	evt := &event.Event{Type: event.EventMessage, Content: event.Content{Raw: raw}}
	assert.Equal(t, "$thread-root", extractThreadID(evt))
	content, attachment, ok := extractInboundMessage(evt, testIDMapper)
	require.True(t, ok)
	assert.Equal(t, "look at this", content)
	require.NotNil(t, attachment)
	assert.Equal(t, "sunset.png", attachment.DisplayName)
}

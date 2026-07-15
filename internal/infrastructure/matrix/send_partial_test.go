package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
)

// ============================================================================
// F5 — partial multi-send semantics
// ============================================================================

// Text lands, then the (single) attachment fails at upload. SendMessage must
// report the delivered text event id AND a PartialSendError, so the caller can
// record what landed instead of treating the whole send as failed.
func TestSendMessage_PartialFailure_ReturnsDeliveredEventAndPartialError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("PDF"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		sendMessageEventResult: &mautrix.RespSendEvent{EventID: "$text1"}, // text succeeds
		uploadBytesErr:         errors.New("synapse upload failed"),       // attachment fails
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	eventID, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"hello with a file",
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "report.pdf", MimeType: "application/pdf", Size: 3}},
		"idem-key",
	)

	require.Error(t, err)
	var partial *domain.PartialSendError
	require.ErrorAs(t, err, &partial, "a mid-sequence failure after delivery must be a PartialSendError")
	assert.Equal(t, "$text1", partial.PrimaryEventID)
	assert.Equal(t, id.EventID("$text1"), eventID, "the delivered primary event id must be returned")
}

// Attachment-only send where the first (and only) event fails: nothing landed,
// so this is a clean TOTAL failure — a raw error, NOT a PartialSendError.
func TestSendMessage_TotalFailure_NotPartial(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("PDF"))
	}))
	defer ts.Close()

	intent := &mockIntentAPI{
		uploadBytesErr: errors.New("synapse upload failed"),
	}
	a := newMediaTestAdapter(t, ts.URL, intent)

	eventID, err := a.SendMessage(
		context.Background(),
		"!room:test.local",
		testActor(testActorID, "Alice"),
		"", // attachment-only
		[]domain.Attachment{{DocumentID: docID1, DisplayName: "report.pdf", MimeType: "application/pdf", Size: 3}},
		"idem-key",
	)

	require.Error(t, err)
	var partial *domain.PartialSendError
	assert.False(t, errors.As(err, &partial), "nothing delivered ⇒ not a partial error")
	assert.Empty(t, eventID)
}

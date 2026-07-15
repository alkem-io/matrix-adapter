package matrix

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/semaphore"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/config"
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

// ============================================================================
// F7 / A5 — attachment memory reservation sized by Content-Length
// ============================================================================

// budgetReservation is a pure function of the response Content-Length and the
// per-attachment cap: a known length within the cap reserves exactly that many
// bytes; an unknown (<= 0) or over-cap length reserves the full cap.
func TestBudgetReservation(t *testing.T) {
	const capBytes = int64(50 * 1024 * 1024)
	assert.Equal(t, int64(1), budgetReservation(1, capBytes), "tiny known length reserves itself")
	assert.Equal(t, int64(1024), budgetReservation(1024, capBytes), "small known length reserves itself")
	assert.Equal(t, capBytes, budgetReservation(capBytes, capBytes), "exactly-cap length reserves the cap")
	assert.Equal(t, capBytes, budgetReservation(capBytes+1, capBytes), "over-cap length reserves the cap")
	assert.Equal(t, capBytes, budgetReservation(-1, capBytes), "unknown (-1) length reserves the cap")
	assert.Equal(t, capBytes, budgetReservation(0, capBytes), "zero (absent) length reserves the cap")
}

// A5 — the total-budget bound still HOLDS for large/unknown-size attachments:
// a fetch with an unknown Content-Length (-1) reserves the full per-attachment
// cap, so of N concurrent acquires only floor(budget/cap) are admitted at once.
// Here budget/cap = 30/10 = 3.
func TestAcquireAttachmentBudget_UnknownLength_BoundsToFloorBudgetOverCap(t *testing.T) {
	a := newTestAdapter("test.local")
	cfg := &config.Config{}
	cfg.FileService.MaxAttachmentBytes = 10      // per-attachment cap (worst-case buffered bytes)
	cfg.FileService.MaxTotalAttachmentBytes = 30 // budget ⇒ floor(30/10) = 3 admitted at once
	a.cfg = cfg
	a.attachmentBudget = semaphore.NewWeighted(cfg.MaxTotalAttachmentBytes())

	const n = 12
	const wantAdmitted = 3

	var inFlight, maxSeen atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Unknown Content-Length (-1) ⇒ reserve the cap ⇒ bound to floor(budget/cap).
			rel, err := a.acquireAttachmentBudget(context.Background(), -1)
			if err != nil {
				return
			}
			defer rel()
			cur := inFlight.Add(1)
			for {
				old := maxSeen.Load()
				if cur <= old || maxSeen.CompareAndSwap(old, cur) {
					break
				}
			}
			<-release
			inFlight.Add(-1)
		}()
	}

	// Only floor(budget/cap) attachments may hold the budget at once.
	require.Eventually(t, func() bool { return maxSeen.Load() == wantAdmitted },
		2*time.Second, 5*time.Millisecond, "budget should admit exactly floor(budget/cap) at once")
	require.Never(t, func() bool { return maxSeen.Load() > wantAdmitted },
		200*time.Millisecond, 10*time.Millisecond,
		"more than floor(budget/cap) admitted — resident memory bound not enforced")

	close(release)
	wg.Wait()
}

// A5 — small attachments (small Content-Length) reserve little, so many more
// than floor(budget/cap) run concurrently instead of being throttled. Here
// budget/cap = 30/10 = 3, but with each fetch reserving only 1 byte all n=12
// acquires are admitted at once (the old cap-always reservation admitted 3).
func TestAcquireAttachmentBudget_SmallLength_FlowsConcurrently(t *testing.T) {
	a := newTestAdapter("test.local")
	cfg := &config.Config{}
	cfg.FileService.MaxAttachmentBytes = 10      // cap ⇒ old design admitted floor(30/10)=3
	cfg.FileService.MaxTotalAttachmentBytes = 30 // budget
	a.cfg = cfg
	a.attachmentBudget = semaphore.NewWeighted(cfg.MaxTotalAttachmentBytes())

	const n = 12 // 12 × 1 byte = 12 <= 30 budget ⇒ all admissible at once

	var inFlight, maxSeen atomic.Int32
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Small known Content-Length (1) ⇒ reserve 1 byte ⇒ not throttled to floor(budget/cap).
			rel, err := a.acquireAttachmentBudget(context.Background(), 1)
			if err != nil {
				return
			}
			defer rel()
			cur := inFlight.Add(1)
			for {
				old := maxSeen.Load()
				if cur <= old || maxSeen.CompareAndSwap(old, cur) {
					break
				}
			}
			<-release
			inFlight.Add(-1)
		}()
	}

	require.Eventually(t, func() bool { return maxSeen.Load() == int32(n) },
		2*time.Second, 5*time.Millisecond,
		"small-length attachments must run concurrently well beyond floor(budget/cap)")

	close(release)
	wg.Wait()
}

// A single max-size attachment must always be admissible: reserving the cap
// against a budget floored at the cap can't deadlock.
func TestAcquireAttachmentBudget_MaxSizeAlwaysAdmissible(t *testing.T) {
	a := newTestAdapter("test.local")
	cfg := &config.Config{}
	cfg.FileService.MaxAttachmentBytes = 64 * 1024 * 1024
	cfg.FileService.MaxTotalAttachmentBytes = 1 // total(1) < per(64MiB): floored up to the cap
	a.cfg = cfg
	assert.Equal(t, int64(64*1024*1024), cfg.MaxTotalAttachmentBytes())

	a.attachmentBudget = semaphore.NewWeighted(cfg.MaxTotalAttachmentBytes())
	// Unknown length ⇒ reserves the cap; must still be admissible.
	rel, err := a.acquireAttachmentBudget(context.Background(), -1)
	require.NoError(t, err, "a single max-size attachment must be admissible")
	rel()
}

package queue

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// The adapter's own execution deadline starts when the handler begins running,
// so it bounds processing and says nothing about how long the request waited in
// the queue first. Under the load this operation is most likely to meet, that
// wait can outlive the caller's RPC timeout — at which point the caller has
// stopped waiting, and has very likely re-read its state and reissued from a
// newer snapshot. Executing the older request then is not late work, it is
// wrong work against state that has moved on.
//
// The caller's absolute expiry closes that window, and it has to be checked
// before any Matrix access so that an expired request is guaranteed to have
// touched nothing — which is what makes it safe for the caller to reissue.

func TestHandleSetChildren_ExpiredRequestIsRejectedBeforeAnyMatrixAccess(t *testing.T) {
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	parentID := uuid.New()
	// Resolvable on purpose: if the handler were to reach Matrix at all, the
	// call would proceed happily. The assertion below is that it never does.
	matrix.resolveAliasResults[otherNewIDMapper().SpaceAlias(parentID)] = "!parent:test.local"

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID:          dto.AlkemioContextID(parentID),
		DesiredChildContextIDs:   []string{uuid.New().String()},
		ApplyRemovals:            true,
		RemovableChildContextIDs: []string{uuid.New().String()},
		ExpiresAtUnixMs:          time.Now().Add(-5 * time.Second).UnixMilli(),
	})

	resp, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, resp, dto.ErrCodeRequestExpired)

	setChildrenResp, ok := resp.(dto.SetChildrenResponse)
	if !ok {
		t.Fatalf("expected a SetChildrenResponse so the caller's normal parsing path still works, got %T", resp)
	}
	if setChildrenResp.Converged {
		t.Error("expected converged=false — an expired request established nothing")
	}
	// Every array must still be present rather than null: a caller that reads
	// these before checking success (the documented pattern) would otherwise
	// throw on the one response shape that is guaranteed to be safe to retry.
	if setChildrenResp.Added == nil || setChildrenResp.Removed == nil ||
		setChildrenResp.UnknownKept == nil || setChildrenResp.ParentPointersUnprocessable == nil {
		t.Errorf("expected every response array normalized to [] on the expiry path, got %+v", setChildrenResp)
	}
}

func TestHandleSetChildren_UnexpiredRequestProceeds(t *testing.T) {
	// Guard against over-correction: a request with plenty of allowance left
	// must be entirely unaffected by the expiry check.
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	parentID := uuid.New()
	matrix.resolveAliasResults[otherNewIDMapper().SpaceAlias(parentID)] = "!parent:test.local"

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID: dto.AlkemioContextID(parentID),
		ExpiresAtUnixMs: time.Now().Add(30 * time.Second).UnixMilli(),
	})

	resp, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	setChildrenResp, ok := resp.(dto.SetChildrenResponse)
	if !ok {
		t.Fatalf("expected a SetChildrenResponse, got %T", resp)
	}
	if !setChildrenResp.Success {
		t.Errorf("expected an unexpired request to run normally, got %+v", setChildrenResp.Error)
	}
}

func TestHandleSetChildren_AbsentExpiryIsNotTreatedAsExpired(t *testing.T) {
	// Zero means "no caller expiry", which is what every caller predating the
	// field sends. Reading that as "expired at the epoch" would reject every
	// one of them.
	matrix := otherNewMatrixPort()
	handler := newTestSpaceHandler(matrix)

	parentID := uuid.New()
	matrix.resolveAliasResults[otherNewIDMapper().SpaceAlias(parentID)] = "!parent:test.local"

	payload := mustMarshal(t, dto.SetChildrenRequest{
		ParentContextID: dto.AlkemioContextID(parentID),
		// ExpiresAtUnixMs deliberately omitted.
	})

	resp, err := handler.HandleSetChildren(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	setChildrenResp, ok := resp.(dto.SetChildrenResponse)
	if !ok {
		t.Fatalf("expected a SetChildrenResponse, got %T", resp)
	}
	if !setChildrenResp.Success {
		t.Errorf("expected a request without an expiry to run normally, got %+v", setChildrenResp.Error)
	}
}

func TestOverdueBy(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		expiresAt int64
		want      time.Duration
	}{
		{"no expiry supplied", 0, 0},
		{"negative expiry is treated as absent", -1, 0},
		{"expiry still ahead", now.Add(3 * time.Second).UnixMilli(), 0},
		{"expiry exactly now is not yet overdue", now.UnixMilli(), 0},
		{"expiry passed", now.Add(-2 * time.Second).UnixMilli(), 2 * time.Second},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := overdueBy(tc.expiresAt, now); got != tc.want {
				t.Errorf("overdueBy(%d) = %v, want %v", tc.expiresAt, got, tc.want)
			}
		})
	}
}

func TestSetChildrenBudget(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	configured := 8 * time.Second

	tests := []struct {
		name      string
		expiresAt int64
		want      time.Duration
	}{
		{
			// Without this, a request that spent most of its allowance queuing
			// would still start a full-length run and could finish writing long
			// after the caller gave up on it.
			name:      "remaining allowance shorter than the configured timeout wins",
			expiresAt: now.Add(2 * time.Second).UnixMilli(),
			want:      2 * time.Second,
		},
		{
			name:      "configured timeout wins when the allowance is generous",
			expiresAt: now.Add(30 * time.Second).UnixMilli(),
			want:      configured,
		},
		{
			name:      "no expiry leaves the configured timeout alone",
			expiresAt: 0,
			want:      configured,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := setChildrenBudget(configured, tc.expiresAt, now); got != tc.want {
				t.Errorf("setChildrenBudget(%v, %d) = %v, want %v", configured, tc.expiresAt, got, tc.want)
			}
		})
	}
}

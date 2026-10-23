package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/testutil"
)

// sessionTokenSentinel is a distinctive value so a log assertion can tell a
// leaked credential from any other string the service happens to log.
const sessionTokenSentinel = "syt_SENTINEL_must_never_be_logged" //nolint:gosec // test sentinel

var sessionTestActorID = uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")

func TestActorService_CreateSession_Success(t *testing.T) {
	minted := &domain.ActorSession{
		UserID:      "@550e8400-e29b-41d4-a716-446655440001:test.local",
		DeviceID:    "DEVICEABC",
		AccessToken: sessionTokenSentinel,
	}
	matrix := &arMockMatrixPort{createSessionResult: minted}
	svc := NewActorService(matrix, &testutil.MockLogger{})

	session, err := svc.CreateSession(context.Background(), sessionTestActorID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if matrix.createSessionCalls != 1 {
		t.Fatalf("expected CreateUserSession to be called once, got %d", matrix.createSessionCalls)
	}
	if matrix.createSessionActor.ID != sessionTestActorID {
		t.Errorf("expected actor %s, got %s", sessionTestActorID, matrix.createSessionActor.ID)
	}
	if *session != *minted {
		t.Errorf("expected the minted triple passed through unchanged, got %+v", session)
	}
}

func TestActorService_CreateSession_PortError(t *testing.T) {
	mintErr := errors.New("homeserver rejected the login")
	matrix := &arMockMatrixPort{createSessionErr: mintErr}
	svc := NewActorService(matrix, &testutil.MockLogger{})

	session, err := svc.CreateSession(context.Background(), sessionTestActorID)
	if !errors.Is(err, mintErr) {
		t.Fatalf("expected the port error, got: %v", err)
	}
	if session != nil {
		t.Errorf("expected no session on error, got %+v", session)
	}
}

func TestActorService_CreateSession_NeverLogsTheToken(t *testing.T) {
	matrix := &arMockMatrixPort{createSessionResult: &domain.ActorSession{
		UserID:      "@550e8400-e29b-41d4-a716-446655440001:test.local",
		DeviceID:    "DEVICEABC",
		AccessToken: sessionTokenSentinel,
	}}
	logger := &testutil.RecordingLogger{}
	svc := NewActorService(matrix, logger)

	if _, err := svc.CreateSession(context.Background(), sessionTestActorID); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	for _, entry := range logger.Entries {
		if strings.Contains(entry, sessionTokenSentinel) {
			t.Fatalf("access token leaked into a log entry: %q", entry)
		}
	}
}

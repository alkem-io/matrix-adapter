package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockActorResolver is a test double for the ActorResolver interface
type mockActorResolver struct {
	resolveActorFn  func(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error)
	resolveEntityFn func(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error)
}

func (m *mockActorResolver) ResolveActorToEntityID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error) {
	if m.resolveActorFn != nil {
		return m.resolveActorFn(ctx, actorID)
	}
	return uuid.Nil, errors.New("not implemented")
}

func (m *mockActorResolver) ResolveEntityToActorID(ctx context.Context, entityID uuid.UUID) (uuid.UUID, error) {
	if m.resolveEntityFn != nil {
		return m.resolveEntityFn(ctx, entityID)
	}
	return uuid.Nil, errors.New("not implemented")
}

func TestIDMapper_UserID_DirectMapping(t *testing.T) {
	// When no ActorResolver is set, UserID should use the actor ID directly as the localpart
	mapper := NewIDMapper("example.com")

	actorID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	ctx := context.Background()

	userID, err := mapper.UserID(ctx, actorID)

	require.NoError(t, err)
	assert.Equal(t, "@12345678-1234-1234-1234-123456789abc:example.com", userID.String())
}

func TestIDMapper_UserID_WithActorResolver(t *testing.T) {
	// When ActorResolver is set, UserID should resolve the actor ID to an entity ID
	mapper := NewIDMapper("example.com")

	actorID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	entityID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")

	resolver := &mockActorResolver{
		resolveActorFn: func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id == actorID {
				return entityID, nil
			}
			return uuid.Nil, NewActorNotFoundError(id.String())
		},
	}

	mapper.SetActorResolver(resolver)

	ctx := context.Background()
	userID, err := mapper.UserID(ctx, actorID)

	require.NoError(t, err)
	// Should use the resolved entity ID, not the actor ID
	assert.Equal(t, "@bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb:example.com", userID.String())
}

func TestIDMapper_UserID_ActorNotFound(t *testing.T) {
	// When ActorResolver fails to find the actor, UserID should return the error
	mapper := NewIDMapper("example.com")

	actorID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	resolver := &mockActorResolver{
		resolveActorFn: func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			return uuid.Nil, NewActorNotFoundError(id.String())
		},
	}

	mapper.SetActorResolver(resolver)

	ctx := context.Background()
	userID, err := mapper.UserID(ctx, actorID)

	require.Error(t, err)
	assert.Empty(t, userID)
	assert.True(t, errors.Is(err, ErrActorNotFound), "error should be ErrActorNotFound")
}

func TestIDMapper_HasActorResolver(t *testing.T) {
	mapper := NewIDMapper("example.com")

	// Initially no resolver
	assert.False(t, mapper.HasActorResolver())

	// Set resolver
	resolver := &mockActorResolver{}
	mapper.SetActorResolver(resolver)
	assert.True(t, mapper.HasActorResolver())

	// Clear resolver
	mapper.SetActorResolver(nil)
	assert.False(t, mapper.HasActorResolver())
}

func TestIDMapper_AlkemioActorID_DirectMapping(t *testing.T) {
	// When no ActorResolver is set, AlkemioActorID should extract the UUID from the localpart directly
	mapper := NewIDMapper("example.com")

	actorID := mapper.AlkemioActorID("@12345678-1234-1234-1234-123456789abc:example.com")

	assert.Equal(t, uuid.MustParse("12345678-1234-1234-1234-123456789abc"), actorID)
}

func TestIDMapper_AlkemioActorID_WithActorResolver(t *testing.T) {
	// When ActorResolver is set, AlkemioActorID should resolve the entity ID to an actor ID
	mapper := NewIDMapper("example.com")

	entityID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	actorID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")

	resolver := &mockActorResolver{
		resolveEntityFn: func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			if id == entityID {
				return actorID, nil
			}
			return uuid.Nil, NewEntityNotFoundError(id.String())
		},
	}

	mapper.SetActorResolver(resolver)

	ctx := context.Background()
	// User ID uses entity ID as localpart (e.g., after forward mapping created the user)
	result, err := mapper.AlkemioActorIDWithContext(ctx, "@bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb:example.com")

	require.NoError(t, err)
	// Should return the resolved actor ID, not the entity ID from the localpart
	assert.Equal(t, actorID, result)
}

func TestIDMapper_AlkemioActorID_EntityNotFound(t *testing.T) {
	// When ActorResolver fails to find the entity, AlkemioActorIDWithContext should return the error
	mapper := NewIDMapper("example.com")

	resolver := &mockActorResolver{
		resolveEntityFn: func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			return uuid.Nil, NewEntityNotFoundError(id.String())
		},
	}

	mapper.SetActorResolver(resolver)

	ctx := context.Background()
	result, err := mapper.AlkemioActorIDWithContext(ctx, "@bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb:example.com")

	require.Error(t, err)
	assert.Equal(t, uuid.Nil, result)
	assert.True(t, errors.Is(err, ErrEntityNotFound), "error should be ErrEntityNotFound")
}

func TestIDMapper_UserID_ReservedUUIDs(t *testing.T) {
	// Reserved UUIDs should bypass ActorResolver and be used directly
	mapper := NewIDMapper("example.com")

	resolverCalled := false
	resolver := &mockActorResolver{
		resolveActorFn: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
			resolverCalled = true
			return uuid.Nil, errors.New("resolver should not be called for reserved UUIDs")
		},
	}
	mapper.SetActorResolver(resolver)

	ctx := context.Background()

	// Test nil UUID
	userID, err := mapper.UserID(ctx, ReservedNilUUID)
	require.NoError(t, err)
	assert.Equal(t, "@00000000-0000-0000-0000-000000000000:example.com", userID.String())
	assert.False(t, resolverCalled, "resolver should not be called for nil UUID")

	// Test bot UUID
	resolverCalled = false
	userID, err = mapper.UserID(ctx, ReservedBotUUID)
	require.NoError(t, err)
	assert.Equal(t, "@ffffffff-ffff-ffff-ffff-fffffffffff0:example.com", userID.String())
	assert.False(t, resolverCalled, "resolver should not be called for bot UUID")
}

func TestIDMapper_AlkemioActorID_ReservedUUIDs(t *testing.T) {
	// Reserved UUIDs should bypass ActorResolver and be returned directly
	mapper := NewIDMapper("example.com")

	resolverCalled := false
	resolver := &mockActorResolver{
		resolveEntityFn: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
			resolverCalled = true
			return uuid.Nil, errors.New("resolver should not be called for reserved UUIDs")
		},
	}
	mapper.SetActorResolver(resolver)

	ctx := context.Background()

	// Test nil UUID
	actorID, err := mapper.AlkemioActorIDWithContext(ctx, "@00000000-0000-0000-0000-000000000000:example.com")
	require.NoError(t, err)
	assert.Equal(t, ReservedNilUUID, actorID)
	assert.False(t, resolverCalled, "resolver should not be called for nil UUID")

	// Test bot UUID
	resolverCalled = false
	actorID, err = mapper.AlkemioActorIDWithContext(ctx, "@ffffffff-ffff-ffff-ffff-fffffffffff0:example.com")
	require.NoError(t, err)
	assert.Equal(t, ReservedBotUUID, actorID)
	assert.False(t, resolverCalled, "resolver should not be called for bot UUID")
}

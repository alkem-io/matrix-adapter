package queue

import (
	"context"

	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter/internal/core/domain"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// AliasResolver resolves Alkemio IDs to Matrix room IDs.
// Shared by all handlers to avoid duplicating resolution logic.
type AliasResolver struct {
	matrix   ports.MatrixPort
	idMapper *domain.IDMapper
}

// NewAliasResolver creates a new AliasResolver.
func NewAliasResolver(matrix ports.MatrixPort, idMapper *domain.IDMapper) *AliasResolver {
	return &AliasResolver{matrix: matrix, idMapper: idMapper}
}

// ResolveRoom resolves an Alkemio room ID to a Matrix room ID.
func (r *AliasResolver) ResolveRoom(ctx context.Context, alkemioRoomID dto.AlkemioRoomID) (id.RoomID, *dto.BaseResponse) {
	alias := r.idMapper.RoomAlias(alkemioRoomID.UUID())
	roomID, err := r.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		if domain.IsNotFoundError(err) {
			resp := NewRoomNotFoundError(alkemioRoomID.String())
			return "", &resp
		}
		resp := MapServiceError(err)
		return "", &resp
	}
	return roomID, nil
}

// ResolveRoomForBatch resolves an Alkemio room ID for batch operations (returns error instead of response).
func (r *AliasResolver) ResolveRoomForBatch(ctx context.Context, alkemioRoomID dto.AlkemioRoomID) (id.RoomID, error) {
	alias := r.idMapper.RoomAlias(alkemioRoomID.UUID())
	return r.matrix.ResolveAlias(ctx, alias)
}

// ResolveSpace resolves an Alkemio context ID to a Matrix room ID.
func (r *AliasResolver) ResolveSpace(ctx context.Context, alkemioContextID dto.AlkemioContextID) (id.RoomID, *dto.BaseResponse) {
	alias := r.idMapper.SpaceAlias(alkemioContextID.UUID())
	roomID, err := r.matrix.ResolveAlias(ctx, alias)
	if err != nil {
		if domain.IsNotFoundError(err) {
			resp := NewSpaceNotFoundError(alkemioContextID.String())
			return "", &resp
		}
		resp := MapServiceError(err)
		return "", &resp
	}
	return roomID, nil
}

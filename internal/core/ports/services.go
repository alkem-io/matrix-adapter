package ports

import (
	"context"

	"github.com/google/uuid"
	"maunium.net/go/mautrix/id"

	"github.com/alkem-io/matrix-adapter-go/internal/core/domain"
)

// ReadReceiptServicePort defines the interface for read receipt service operations.
type ReadReceiptServicePort interface {
	// MarkMessageRead marks a message as read for a user.
	// If threadRootID is provided, it marks a thread-level receipt.
	MarkMessageRead(
		ctx context.Context,
		actorID uuid.UUID,
		roomID id.RoomID,
		eventID id.EventID,
		threadRootID *id.EventID,
	) error

	// GetUnreadCounts retrieves unread message counts for a user in a room.
	// If threadRootIDs is provided, it returns counts for those specific threads.
	GetUnreadCounts(
		ctx context.Context,
		actorID uuid.UUID,
		roomID id.RoomID,
		threadRootIDs []id.EventID,
	) (*domain.UnreadCountSummary, error)
}

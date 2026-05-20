package domain

import "maunium.net/go/mautrix/id"

// RoomCheckRequest represents a parsed room creation check request in the domain layer.
type RoomCheckRequest struct {
	Creator  id.UserID
	Members  []id.UserID
	IsDirect bool
}

// RoomCheckResponse represents the result of a room creation check.
type RoomCheckResponse struct {
	Allow         bool
	AlkemioRoomID string
	Reason        string
}

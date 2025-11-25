package dto

// BaseMatrixAdapterEventPayload represents the common fields for all Matrix adapter events.
type BaseMatrixAdapterEventPayload struct {
	// The Actor ID (UUID) of the user who triggered the event.
	TriggeredBy string `json:"triggeredBy"`
}

package dto

// ActorRegisterPayload represents the payload for registering an actor.
type ActorRegisterPayload struct {
	BaseMatrixAdapterEventPayload
	ActorID     string `json:"actorId"`
	DisplayName string `json:"displayName"`
}

// ActorRegisterResponse represents the response after registering an actor.
type ActorRegisterResponse struct {
	Success  bool   `json:"success"`
	MatrixID string `json:"matrixId"`
}

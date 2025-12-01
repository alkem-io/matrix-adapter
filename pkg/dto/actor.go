package dto

// ActorRegisterPayload represents the payload for registering an actor.
type ActorRegisterPayload struct {
	BaseMatrixAdapterEventPayload
	ActorID     string `json:"actorId"`
	DisplayName string `json:"displayName"`
}

// ActorRegisterResponsePayload represents the response after registering an actor.
type ActorRegisterResponsePayload struct {
	BaseResponse
	MatrixID string `json:"matrixId"`
}

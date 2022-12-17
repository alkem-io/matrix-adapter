
import { BaseEventPayload } from './base.event.payload';

export interface ReplicateRoomMembershipPayload extends BaseEventPayload {
  targetRoomID: string;
  sourceRoomID: string;
  userToPrioritize: string;
}

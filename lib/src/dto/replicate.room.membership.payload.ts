
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface ReplicateRoomMembershipPayload extends BaseMatrixAdapterEventPayload {
  targetRoomID: string;
  sourceRoomID: string;
  userToPrioritize: string;
}

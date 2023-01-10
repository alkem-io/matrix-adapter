
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface ReplicateRoomMembershipResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  success: boolean;
}

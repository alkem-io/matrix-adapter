
import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface ReplicateRoomMembershipResponsePayload extends BaseEventResponsePayload {
  success: boolean;
}

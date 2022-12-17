
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomMembersResponsePayload extends BaseEventResponsePayload {
  userIDs: string[];
}

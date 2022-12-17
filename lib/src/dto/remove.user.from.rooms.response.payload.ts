
import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RemoveUserFromRoomsResponsePayload extends BaseEventResponsePayload {
  success: boolean;
}


import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RemoveRoomResponsePayload extends BaseEventResponsePayload {
  success: boolean;
}

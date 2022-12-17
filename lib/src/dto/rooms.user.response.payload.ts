
import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomsUserResponsePayload extends BaseEventResponsePayload {
  rooms: RoomResult[];
}

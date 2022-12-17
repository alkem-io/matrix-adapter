
import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomsUserDirectResponsePayload extends BaseEventResponsePayload {
  rooms: RoomResult[];
}


import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomsResponsePayload extends BaseEventResponsePayload {
  rooms: RoomResult[];
}

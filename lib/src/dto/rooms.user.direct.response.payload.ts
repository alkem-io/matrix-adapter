
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomsUserDirectResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  rooms: RoomResult[];
}

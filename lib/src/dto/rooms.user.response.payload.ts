
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomsUserResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  rooms: RoomResult[];
}

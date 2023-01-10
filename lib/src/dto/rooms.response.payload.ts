
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomsResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  rooms: RoomResult[];
}

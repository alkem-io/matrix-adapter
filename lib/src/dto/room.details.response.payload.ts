
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomDetailsResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  room: RoomResult;
}

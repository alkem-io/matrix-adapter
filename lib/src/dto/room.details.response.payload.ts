
import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomDetailsResponsePayload extends BaseEventResponsePayload {
  room: RoomResult;
}

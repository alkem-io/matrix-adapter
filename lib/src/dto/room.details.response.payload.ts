
import { RoomResult } from '..';
import { BaseEventPayload } from './base.event.payload';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomDetailsResponsePayload extends BaseEventResponsePayload {
  room: RoomResult;
}

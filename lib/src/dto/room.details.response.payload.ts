
import { RoomResult } from '..';
import { BaseEventPayload } from './base.event.payload';

export interface RoomDetailsResponsePayload extends BaseEventPayload {
  room: RoomResult;
}

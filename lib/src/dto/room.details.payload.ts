
import { BaseEventPayload } from './base.event.payload';

export interface RoomDetailsPayload extends BaseEventPayload {
  roomID: string;
}

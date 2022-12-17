
import { BaseEventPayload } from './base.event.payload';

export interface RoomDeleteMessagePayload extends BaseEventPayload {
  roomID: string;
  senderID: string;
  messageID: string;
}

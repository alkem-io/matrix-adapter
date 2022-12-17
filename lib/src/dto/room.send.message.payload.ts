
import { BaseEventPayload } from './base.event.payload';

export interface RoomSendMessagePayload extends BaseEventPayload {
  roomID: string;
  senderID: string;
  message: string;
}

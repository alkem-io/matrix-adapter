
import { BaseEventPayload } from './base.event.payload';

export interface RoomMessageSenderPayload extends BaseEventPayload {
  roomID: string;
  messageID: string;
}


import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomDeleteMessagePayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
  senderID: string;
  messageID: string;
}

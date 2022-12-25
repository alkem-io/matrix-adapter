
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomMessageSenderPayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
  messageID: string;
}

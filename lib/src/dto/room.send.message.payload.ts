
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomSendMessagePayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
  senderID: string;
  message: string;
}

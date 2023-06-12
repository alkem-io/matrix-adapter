import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomSendMessageReplyPayload
  extends BaseMatrixAdapterEventPayload {
  roomID: string;
  senderID: string;
  threadID: string;
  message: string;
}

import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomAddMessageReactionPayload
  extends BaseMatrixAdapterEventPayload {
  roomID: string;
  senderID: string;
  messageID: string;
  emoji: string;
}

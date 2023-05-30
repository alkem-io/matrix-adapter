import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomRemoveMessageReactionPayload
  extends BaseMatrixAdapterEventPayload {
  roomID: string;
  senderID: string;
  reactionID: string;
}

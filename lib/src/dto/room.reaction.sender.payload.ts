import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomReactionSenderPayload
  extends BaseMatrixAdapterEventPayload {
  roomID: string;
  reactionID: string;
}

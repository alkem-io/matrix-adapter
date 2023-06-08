import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomReactionSenderResponsePayload
  extends BaseMatrixAdapterEventResponsePayload {
  senderID: string;
}

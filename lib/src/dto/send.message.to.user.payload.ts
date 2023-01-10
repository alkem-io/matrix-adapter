
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface SendMessageToUserPayload extends BaseMatrixAdapterEventPayload {
  receiverID: string;
  senderID: string;
  message: string;
}

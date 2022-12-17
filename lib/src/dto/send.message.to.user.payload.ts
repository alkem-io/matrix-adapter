
import { BaseEventPayload } from './base.event.payload';

export interface SendMessageToUserPayload extends BaseEventPayload {
  receiverID: string;
  senderID: string;
  message: string;
}

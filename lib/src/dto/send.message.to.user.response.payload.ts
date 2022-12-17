
import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface SendMessageToUserResponsePayload extends BaseEventResponsePayload {
  messageID: string;
}

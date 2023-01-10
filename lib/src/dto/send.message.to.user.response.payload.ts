
import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface SendMessageToUserResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  messageID: string;
}

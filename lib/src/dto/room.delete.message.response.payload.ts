
import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomDeleteMessageResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  messageID: string;
}


import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomSendMessageResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  message: IMessage;
}

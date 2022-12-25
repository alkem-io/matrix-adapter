
import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomMessageSenderResponsePayload extends BaseMatrixAdapterEventResponsePayload {
   senderID: string;
}

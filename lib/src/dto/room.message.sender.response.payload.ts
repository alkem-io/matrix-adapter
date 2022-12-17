
import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomMessageSenderResponsePayload extends BaseEventResponsePayload {
   senderID: string;
}

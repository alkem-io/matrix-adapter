
import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomSendMessageResponsePayload extends BaseEventResponsePayload {
  message: IMessage;
}


import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomDeleteMessageResponsePayload extends BaseEventResponsePayload {
  messageID: string;
}

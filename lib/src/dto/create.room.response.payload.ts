
import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface CreateRoomResponsePayload extends BaseEventResponsePayload {
  roomID: string;
}


import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface AddUserToRoomResponsePayload extends BaseEventResponsePayload {
  success: boolean;
}


import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface AddUserToRoomsResponsePayload extends BaseEventResponsePayload {
  success: boolean;
}

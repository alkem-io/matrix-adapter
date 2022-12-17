
import { RoomResult } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RegisterNewUserResponsePayload extends BaseEventResponsePayload {
  userID?: string;
}


import { BaseEventPayload } from './base.event.payload';

export interface AddUserToRoomPayload extends BaseEventPayload {
  roomID: string;
  userID: string;
}

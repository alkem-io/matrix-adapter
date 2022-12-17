
import { BaseEventPayload } from './base.event.payload';

export interface AddUserToRoomsPayload extends BaseEventPayload {
  groupID: string;
  roomIDs: string[];
  userID: string;
}

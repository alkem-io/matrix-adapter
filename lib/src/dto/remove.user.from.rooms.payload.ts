
import { BaseEventPayload } from './base.event.payload';

export interface RemoveUserFromRoomsPayload extends BaseEventPayload {
  userID: string;
  roomIDs: string[];
}

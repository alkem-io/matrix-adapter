
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface AddUserToRoomsPayload extends BaseMatrixAdapterEventPayload {
  groupID: string;
  roomIDs: string[];
  userID: string;
}

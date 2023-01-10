
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RemoveUserFromRoomsPayload extends BaseMatrixAdapterEventPayload {
  userID: string;
  roomIDs: string[];
}

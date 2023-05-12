
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface AddUserToRoomsPayload extends BaseMatrixAdapterEventPayload {
  roomIDs: string[];
  userID: string;
}

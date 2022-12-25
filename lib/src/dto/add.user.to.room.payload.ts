
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface AddUserToRoomPayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
  userID: string;
}

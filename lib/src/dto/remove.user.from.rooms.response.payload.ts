
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RemoveUserFromRoomsResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  success: boolean;
}

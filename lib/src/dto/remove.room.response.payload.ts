
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RemoveRoomResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  success: boolean;
}

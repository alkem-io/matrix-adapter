
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RemoveRoomPayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
}


import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface UpdateRoomStatePayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
  
  historyWorldVisibile?: boolean;
  allowJoining?: boolean;
}

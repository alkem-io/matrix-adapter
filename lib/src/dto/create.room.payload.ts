
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface CreateRoomPayload extends BaseMatrixAdapterEventPayload {
  groupID: string;
  roomName: string;
  metadata?: Record<string, string>
}

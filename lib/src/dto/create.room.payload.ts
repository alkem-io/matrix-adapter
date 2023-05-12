
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface CreateRoomPayload extends BaseMatrixAdapterEventPayload {
  roomName: string;
  metadata?: Record<string, string>
}

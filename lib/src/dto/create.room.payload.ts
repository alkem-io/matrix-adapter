
import { BaseEventPayload } from './base.event.payload';

export interface CreateRoomPayload extends BaseEventPayload {
  groupID: string;
  roomName: string;
  metadata?: Record<string, string>
}

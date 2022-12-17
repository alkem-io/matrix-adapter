
import { BaseEventPayload } from './base.event.payload';

export interface RemoveRoomPayload extends BaseEventPayload {
  roomID: string;
}

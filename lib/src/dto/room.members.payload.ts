
import { BaseEventPayload } from './base.event.payload';

export interface RoomMembersPayload extends BaseEventPayload {
  roomID: string;
}

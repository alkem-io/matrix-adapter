
import { BaseEventPayload } from './base.event.payload';

export interface RoomJoinRulePayload extends BaseEventPayload {
  roomID: string;
}

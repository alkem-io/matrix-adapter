
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomJoinRulePayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
}

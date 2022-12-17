
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface RoomJoinRuleResponsePayload extends BaseEventResponsePayload {
  rule: string;
}

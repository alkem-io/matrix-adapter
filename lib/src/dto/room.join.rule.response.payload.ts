
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomJoinRuleResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  rule: string;
}

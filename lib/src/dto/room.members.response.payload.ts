
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomMembersResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  userIDs: string[];
}

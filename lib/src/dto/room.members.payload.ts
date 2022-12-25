
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomMembersPayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
}

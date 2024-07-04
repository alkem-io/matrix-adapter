
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomDetailsPayload extends BaseMatrixAdapterEventPayload {
  roomID: string;
  withState?: boolean;
}

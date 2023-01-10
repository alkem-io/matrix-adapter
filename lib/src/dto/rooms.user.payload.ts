
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomsUserPayload extends BaseMatrixAdapterEventPayload {
  userID: string;
}


import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RoomsUserDirectPayload extends BaseMatrixAdapterEventPayload {
  userID: string;
}

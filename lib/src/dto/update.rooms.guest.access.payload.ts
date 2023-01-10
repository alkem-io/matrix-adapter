
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface UpdateRoomsGuestAccessPayload extends BaseMatrixAdapterEventPayload {
  roomIDs: string[];
  allowGuests: boolean;
}

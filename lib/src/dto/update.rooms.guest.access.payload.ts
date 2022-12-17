
import { BaseEventPayload } from './base.event.payload';

export interface UpdateRoomsGuestAccessPayload extends BaseEventPayload {
  roomIDs: string[];
  allowGuests: boolean;
}

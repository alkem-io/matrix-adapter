
import { BaseEventPayload } from './base.event.payload';

export interface RoomsUserPayload extends BaseEventPayload {
  userID: string;
}

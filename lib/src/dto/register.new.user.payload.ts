
import { BaseEventPayload } from './base.event.payload';

export interface RegisterNewUserPayload extends BaseEventPayload {
  email: string;
}

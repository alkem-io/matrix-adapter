
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface RegisterNewUserPayload extends BaseMatrixAdapterEventPayload {
  email: string;
}

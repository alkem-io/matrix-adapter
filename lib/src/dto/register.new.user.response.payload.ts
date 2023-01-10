
import { RoomResult } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RegisterNewUserResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  userID?: string;
}

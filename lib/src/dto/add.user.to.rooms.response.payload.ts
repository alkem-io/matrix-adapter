
import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface AddUserToRoomsResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  success: boolean;
}

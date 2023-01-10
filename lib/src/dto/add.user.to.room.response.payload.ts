
import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface AddUserToRoomResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  success: boolean;
}

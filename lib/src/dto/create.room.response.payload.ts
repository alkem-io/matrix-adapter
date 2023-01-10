
import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface CreateRoomResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  roomID: string;
}

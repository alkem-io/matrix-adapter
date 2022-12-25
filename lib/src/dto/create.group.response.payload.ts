
import { IMessage } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface CreateGroupResponsePayload extends BaseMatrixAdapterEventResponsePayload {
  groupID: string;
}


import { IMessage } from '..';
import { BaseEventResponsePayload } from './base.event.response.payload';

export interface CreateGroupResponsePayload extends BaseEventResponsePayload {
  groupID: string;
}

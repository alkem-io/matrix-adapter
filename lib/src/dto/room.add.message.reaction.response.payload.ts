import { IReaction } from '..';
import { BaseMatrixAdapterEventResponsePayload } from './base.event.response.payload';

export interface RoomAddMessageReactionResponsePayload
  extends BaseMatrixAdapterEventResponsePayload {
  reaction: IReaction;
}

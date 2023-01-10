
import { BaseMatrixAdapterEventPayload } from './base.event.payload';

export interface CreateGroupPayload extends BaseMatrixAdapterEventPayload {
  communityID: string;
  communityDisplayName: string;
}


import { BaseEventPayload } from './base.event.payload';

export interface CreateGroupPayload extends BaseEventPayload {
  communityID: string;
  communityDisplayName: string;
}

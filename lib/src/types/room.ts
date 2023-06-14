import { IMessage } from './message';

export class RoomResult {
  id!: string;

  messages!: IMessage[];

  displayName!: string;

  // The communication IDs of the room members
  members!: string[];
}

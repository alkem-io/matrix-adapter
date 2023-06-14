import { IReaction } from './reaction';

export class IMessage {
  id!: string;

  message!: string;

  threadID?: string;

  sender!: string;

  timestamp!: number;

  reactions!: IReaction[];
}

import { IMessage } from '@alkemio/matrix-adapter-lib';

export class CommunicationEventMessageReceived {
  roomId!: string;

  roomName!: string;

  message!: IMessage;

  communicationID!: string;

  communityId!: string | undefined;
}

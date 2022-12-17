import { Controller, Inject, LoggerService } from '@nestjs/common';
import {
  Ctx,
  MessagePattern,
  Payload,
  RmqContext,
  RpcException,
} from '@nestjs/microservices';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { LogContext } from './common/enums';
import {
  MatrixAdapterEventType,
  RoomDetailsResponsePayload,
} from '@alkemio/matrix-adapter-lib';
import { RoomDetailsPayload } from '@alkemio/matrix-adapter-lib';
import { CommunicationAdapter } from './services/communication-adapter/communication.adapter';
import { RoomSendMessagePayload } from '@alkemio/matrix-adapter-lib';
import { RoomSendMessageResponsePayload } from '@alkemio/matrix-adapter-lib';

@Controller()
export class AppController {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService,
    private communicationAdapter: CommunicationAdapter
  ) {}

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOM_DETAILS })
  async roomDetails(
    @Payload() data: RoomDetailsPayload,
    @Ctx() context: RmqContext
  ): Promise<RoomDetailsResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ROOM_DETAILS} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const room = await this.communicationAdapter.getCommunityRoom(
        data.roomID
      );
      channel.ack(originalMsg);
      const response: RoomDetailsResponsePayload = {
        room: room,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when getting room details: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern({ cmd: MatrixAdapterEventType.ROOM_SEND_MESSAGE })
  async roomSendMessage(
    @Payload() data: RoomSendMessagePayload,
    @Ctx() context: RmqContext
  ): Promise<RoomSendMessageResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdapterEventType.ROOM_SEND_MESSAGE} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.COMMUNICATION
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      const message = await this.communicationAdapter.sendMessage(data);
      channel.ack(originalMsg);
      const response: RoomSendMessageResponsePayload = {
        message: message,
      };

      return response;
    } catch (error) {
      const errorMessage = `Error when sending message to room: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }
}

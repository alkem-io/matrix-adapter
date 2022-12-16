import { Controller, Inject, LoggerService } from '@nestjs/common';
import { Ctx, EventPattern, Payload, RmqContext } from '@nestjs/microservices';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { Channel, Message } from 'amqplib';
import { LogContext } from './common/enums';
import { MatrixAdapterEventType } from '@alkemio/matrix-adapter-lib';
import {
  BaseEventPayload,
  RoomDetailsPayload,
} from '@alkemio/matrix-adapter-lib';

@Controller()
export class AppController {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService
  ) {}

  @EventPattern(MatrixAdapterEventType.ROOM_DETAILS)
  async roomDetails(
    // todo is auto validation possible
    @Payload() eventPayload: RoomDetailsPayload,
    @Ctx() context: RmqContext
  ) {
    this.roomDetailsImpl(
      eventPayload,
      context,
      MatrixAdapterEventType.ROOM_DETAILS
    );
  }

  private async roomDetailsImpl(
    @Payload() eventPayload: BaseEventPayload,
    @Ctx() context: RmqContext,
    eventName: string
  ) {
    this.logger.verbose?.(
      `[Event received: ${eventName}]: ${JSON.stringify(eventPayload)}`,
      LogContext.MATRIX
    );

    const channel: Channel = context.getChannelRef();
    const originalMsg = context.getMessage() as Message;

    channel.ack(originalMsg);
  }
}

import { Controller, Inject, LoggerService } from '@nestjs/common';
import {
  Ctx,
  MessagePattern,
  Payload,
  RmqContext,
  RpcException,
  Transport,
} from '@nestjs/microservices';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { LogContext } from '../../common/enums/index.js';
import { MatrixAdminEventType } from './matrix.admin.event.type.js';
import { MatrixAdminEventUpdateRoomStateForAdminRoomsInput } from './dto/matrix.admin.dto.event.update.room.state.for.admin.rooms.js';
import { MatrixAdminService } from './matrix.admin.service.js';
import { MatrixAdminBaseEventResponsePayload } from './dto/matrix.admin.base.event.response.payload.js';
import { MatrixAdminEventLogRoomStateInput } from './dto/matrix.admin.dto.event.log.room.state.js';

@Controller()
export class MatrixAdminController {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService,
    private matrixAdminService: MatrixAdminService
  ) {}

  @MessagePattern('updateRoomStateForAdminRooms', Transport.RMQ)
  async matrixAdminRoomsReset(
    @Payload() data: MatrixAdminEventUpdateRoomStateForAdminRoomsInput,
    @Ctx() context: RmqContext
  ): Promise<MatrixAdminBaseEventResponsePayload> {
    this.logger.verbose?.(
      `${
        MatrixAdminEventType.UPDATE_ROOM_STATE_FOR_ADMIN_ROOMS
      } - payload: ${JSON.stringify(data)}`,
      LogContext.MATRIX_ADMIN_EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      await this.matrixAdminService.updateRoomStateForAdminRooms(data);
      channel.ack(originalMsg);
      const response: MatrixAdminBaseEventResponsePayload = {};

      return response;
    } catch (error) {
      const errorMessage = `Error when resetting matrix rooms for admin: ${error}, payload: ${JSON.stringify(
        data
      )}`;
      this.logger.error(errorMessage, LogContext.MATRIX_ADMIN);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }

  @MessagePattern('logRoomState', Transport.RMQ)
  async matrixAdminRoomState(
    @Payload() data: MatrixAdminEventLogRoomStateInput,
    @Ctx() context: RmqContext
  ): Promise<MatrixAdminBaseEventResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdminEventType.LOG_ROOM_STATE} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.MATRIX_ADMIN_EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      await this.matrixAdminService.logRoomState(data);
      channel.ack(originalMsg);
      const response: MatrixAdminBaseEventResponsePayload = {};

      return response;
    } catch (error) {
      const errorMessage = `Error when resetting matrix rooms for admin: ${error}, payload: ${JSON.stringify(
        data
      )}`;
      this.logger.error(errorMessage, LogContext.MATRIX_ADMIN);
      channel.ack(originalMsg);
      throw new RpcException(errorMessage);
    }
  }
}

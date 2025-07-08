import pkg  from '@nestjs/common';
const { Inject, Controller } = pkg;
import {
  Ctx,
  MessagePattern,
  Payload,
  RmqContext,
  RpcException,
  Transport,
} from '@nestjs/microservices';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { LogContext } from '../../common/enums/index';
import { MatrixAdminEventType } from './matrix.admin.event.type';
import { MatrixAdminEventUpdateRoomStateForAdminRoomsInput } from './dto/matrix.admin.dto.event.update.room.state.for.admin.rooms';
import { MatrixAdminService } from './matrix.admin.service';
import { MatrixAdminBaseEventResponsePayload } from './dto/matrix.admin.base.event.response.payload';
import { MatrixAdminEventLogRoomStateInput } from './dto/matrix.admin.dto.event.log.room.state';

@Controller()
export class MatrixAdminController {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private matrixAdminService: MatrixAdminService
  ) {}

  @MessagePattern(MatrixAdminEventType.UPDATE_ROOM_STATE_FOR_ADMIN_ROOMS, Transport.RMQ)
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

  @MessagePattern(MatrixAdminEventType.LOG_ROOM_STATE, Transport.RMQ)
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

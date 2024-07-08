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
import { LogContext } from '../../common/enums';
import { MatrixAdminEventType } from './matrix.admin.event.type';
import { MatrixAdminEventUpdateRoomStateForAdminRoomsInput } from './dto/matrix.admin.dto.event.update.room.state.for.admin.rooms';
import { MatrixAdminService } from './matrix.admin.service';
import { MatrixAdminBaseEventResponsePayload } from './dto/matrix.admin.base.event.response.payload';
import { MatrixAdminEventLogRoomStateInput } from './dto/matrix.admin.dto.event.log.room.state';

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
    channel.ack(originalMsg);

    try {
      await this.matrixAdminService.updateRoomStateForAdminRooms(data);
      return {};
    } catch (error: any) {
      const errorMessage = `Error when resetting matrix rooms for admin '${
        data.adminEmail
      }': ${error?.message ?? error}, payload: ${JSON.stringify(data)}`;
      this.logger.error(errorMessage, undefined, LogContext.MATRIX_ADMIN);
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
    channel.ack(originalMsg);

    try {
      await this.matrixAdminService.logRoomState(data);
      return {};
    } catch (error: any) {
      const errorMessage = `Error when rooms for admin: ${
        error?.message ?? error
      }, payload: ${JSON.stringify(data)}`;
      this.logger.error(errorMessage, error?.stack, LogContext.MATRIX_ADMIN);
      throw new RpcException(errorMessage);
    }
  }
}

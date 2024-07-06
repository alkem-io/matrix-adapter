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
import { MatrixAdminEventResetAdminRoomsInput } from './dto/matrix.admin.dto.event.reset.admin.rooms';
import { MatrixAdminService } from './matrix.admin.service';
import { MatrixAdminBaseEventResponsePayload } from './dto/matrix.admin.base.event.response.payload';

@Controller()
export class MatrixAdminController {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService,
    private matrixAdminService: MatrixAdminService
  ) {}

  @MessagePattern('adminRoomsReset', Transport.RMQ)
  async matrixAdminRoomsReset(
    @Payload() data: MatrixAdminEventResetAdminRoomsInput,
    @Ctx() context: RmqContext
  ): Promise<MatrixAdminBaseEventResponsePayload> {
    this.logger.verbose?.(
      `${MatrixAdminEventType.ADMIN_ROOMS_RESET} - payload: ${JSON.stringify(
        data
      )}`,
      LogContext.MATRIX_ADMIN_EVENTS
    );
    const channel = context.getChannelRef();
    const originalMsg = context.getMessage();

    try {
      await this.matrixAdminService.updatePowerLevelsInRoomsForAdmin(data);
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

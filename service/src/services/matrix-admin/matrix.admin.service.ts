import { Inject, Injectable, LoggerService } from '@nestjs/common';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { CommunicationAdapter } from '../communication-adapter/communication.adapter';
import { LogContext } from '@src/common/enums/logging.context';
import { RoomPowerLevelsEventContent } from 'matrix-js-sdk/lib/types';
import { MatrixEntityNotFoundException } from '@src/common/exceptions/matrix.entity.not.found.exception';
import { EventType, IStateEventWithRoomId } from 'matrix-js-sdk';
import { MatrixAdminEventResetAdminRoomsInput } from './dto/matrix.admin.dto.event.reset.admin.rooms';
import { IOperationalMatrixUser } from '../matrix/adapter-user/matrix.user.interface';
import { MatrixUserManagementService } from '../matrix/management/matrix.user.management.service';
import { MatrixAgentService } from '../matrix/agent/matrix.agent.service';
import { MatrixUserAdapter } from '../matrix/adapter-user/matrix.user.adapter';

@Injectable()
export class MatrixAdminService {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: LoggerService,
    private communicationAdapter: CommunicationAdapter,
    private matrixUserManagementService: MatrixUserManagementService,
    private matrixAgentService: MatrixAgentService,
    private matrixUserAdapter: MatrixUserAdapter
  ) {}

  private async getPowerLevelsEventForRoom(
    roomID: string
  ): Promise<IStateEventWithRoomId> {
    const matrixAgentElevated =
      await this.communicationAdapter.getMatrixManagementAgentElevated();
    const matrixClient = matrixAgentElevated.matrixClient;
    const roomState = await matrixClient.roomState(roomID);
    // Find the user's power level event (usually type "m.room.power_levels")
    const powerLevelsEvent = roomState.find(
      event => event.type === 'm.room.power_levels'
    );
    if (!powerLevelsEvent) {
      throw new MatrixEntityNotFoundException(
        `Unable to retrieve power level for admin in room: ${roomID}`,
        LogContext.COMMUNICATION
      );
    }
    this.logRoomPowerLevelsEvent(powerLevelsEvent);
    return powerLevelsEvent;
  }

  private logRoomPowerLevelsEvent(powerLevelsEvent: IStateEventWithRoomId) {
    const powerLevelsEventUsers = powerLevelsEvent.content.users;
    const poserLevelsEventDefaultUser = powerLevelsEvent.content.users_default;
    this.logger.verbose?.(
      `Room (${
        powerLevelsEvent.room_id
      }) - power levels event sent by: ${JSON.stringify(
        powerLevelsEventUsers
      )} - default users level: ${poserLevelsEventDefaultUser}`,
      LogContext.COMMUNICATION
    );
  }

  private async createMatrixClientForAdmin(adminUser: IOperationalMatrixUser) {
    const adminAgent = await this.matrixAgentService.createMatrixAgent(
      adminUser
    );

    await adminAgent.start({
      registerTimelineMonitor: false,
      registerRoomMonitor: false,
    });

    return adminAgent;
  }

  private async getGlobalAdminUser(
    username: string,
    password: string
  ): Promise<IOperationalMatrixUser> {
    const adminCommunicationsID =
      this.matrixUserAdapter.convertEmailToMatrixID(username);
    const adminExists = await this.matrixUserManagementService.isRegistered(
      adminCommunicationsID
    );
    if (!adminExists) {
      // trying to reset an admin that is not known!
      throw new MatrixEntityNotFoundException(
        `trying to reset an admin that is not known: ${username}`,
        LogContext.COMMUNICATION
      );
    }
    this.logger.verbose?.(
      `Admin user is registered: ${username}, logging in...`,
      LogContext.COMMUNICATION
    );
    const adminUser = await this.matrixUserManagementService.login(
      adminCommunicationsID,
      password
    );
    return adminUser;
  }

  public async updatePowerLevelsInRoomsForAdmin(
    resetAdminRoomsData: MatrixAdminEventResetAdminRoomsInput
  ) {
    this.logger.verbose?.(
      `*** Resetting power level in rooms for admin: ${resetAdminRoomsData.adminEmail} ***`,
      LogContext.COMMUNICATION
    );
    const adminUser = await this.getGlobalAdminUser(
      resetAdminRoomsData.adminEmail,
      resetAdminRoomsData.adminPassword
    );
    const adminAgent = await this.createMatrixClientForAdmin(adminUser);
    const matrixClient = adminAgent.matrixClient;
    const adminUserID =
      this.communicationAdapter.getUserIdFromMatrixClient(matrixClient);
    const rooms = matrixClient.getRooms();
    let roomsUpdated = 0;
    try {
      for (const room of rooms) {
        const roomID = room.roomId;

        const powerLevelsEvent = await this.getPowerLevelsEventForRoom(roomID);
        const powerLevelsEventUsers = powerLevelsEvent.content.users;

        const userStr = JSON.stringify(powerLevelsEventUsers);
        const newPowerLevelDefaultUser =
          resetAdminRoomsData.powerLevel.users_default;
        if (userStr.includes(adminUserID)) {
          // Update the user's power level (e.g., set them as an admin)
          const updatedPowerLevelsInput: RoomPowerLevelsEventContent = {
            ...powerLevelsEvent.content,
            users_default: newPowerLevelDefaultUser,
          };
          //Send the updated power levels event
          await matrixClient.sendStateEvent(
            roomID,
            EventType.RoomPowerLevels,
            updatedPowerLevelsInput
          );
          roomsUpdated++;
          this.logger.verbose?.(
            `...power level update sent for room ${roomID}`,
            LogContext.COMMUNICATION
          );
          await this.getPowerLevelsEventForRoom(roomID);
        } else {
          this.logger.verbose?.(
            `...power level update skipped as current user is not empowered by: ${JSON.stringify(
              powerLevelsEventUsers
            )}`,
            LogContext.COMMUNICATION
          );
        }
      }
      this.logger.verbose?.(
        `Power levels updated in ${roomsUpdated} rooms for admin: ${resetAdminRoomsData.adminEmail}`,
        LogContext.COMMUNICATION
      );
    } catch (error) {
      const errorMessage = `Unable to update power levels in rooms: ${error}`;
      this.logger.error(errorMessage, LogContext.COMMUNICATION);
      throw error;
    }
  }
}

import pkg  from '@nestjs/common';
const { Inject, Injectable } = pkg;
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { CommunicationAdapter } from '../../communication-adapter/communication.adapter';
import { LogContext } from '@src/common/enums/logging.context';
import { RoomPowerLevelsEventContent } from 'matrix-js-sdk/lib/types';
import { MatrixEntityNotFoundException } from '@src/common/exceptions/matrix.entity.not.found.exception';
import { EventType, IStateEventWithRoomId, MatrixClient } from 'matrix-js-sdk';
import { MatrixAdminEventUpdateRoomStateForAdminRoomsInput as MatrixAdminEventUpdateRoomStateForAdminRoomsInput } from './dto/matrix.admin.roomsdto.event.update.room.state.for.admin.rooms';
import { IOperationalMatrixUser } from '../../matrix/adapter-user/matrix.user.interface';
import { MatrixAgentFactoryService } from '../../matrix/agent-factory/matrix.agent.factory.service';
import { MatrixUserAdapter } from '../../matrix/adapter-user/matrix.user.adapter';
import { MatrixAdminEventLogRoomStateInput } from './dto/matrix.admin.rooms.dto.event.log.room.state';
import { MatrixAdminUserElevatedService } from '../user-elevated/matrix.admin.user.elevated.service';
import { MatrixAdminUserService } from '../user/matrix.admin.user.service';

@Injectable()
export class MatrixAdminRoomsService {
  constructor(
    @Inject(WINSTON_MODULE_NEST_PROVIDER)
    private readonly logger: pkg.LoggerService,
    private communicationAdapter: CommunicationAdapter,
    private matrixUserManagementService: MatrixAdminUserService,
    private matrixAgentService: MatrixAgentFactoryService,
    private matrixUserAdapter: MatrixUserAdapter,
    private communicationAdminUserService: MatrixAdminUserElevatedService
  ) {}

  private async getPowerLevelsEventForRoom(
    matrixClient: MatrixClient,
    roomID: string
  ): Promise<IStateEventWithRoomId> {
    const roomState = await matrixClient.roomState(roomID);
    // Find the user's power level event (usually type "m.room.power_levels")
    const powerLevelsEvent = roomState.find(
      event => event.type === EventType.RoomPowerLevels
    );
    if (!powerLevelsEvent) {
      throw new MatrixEntityNotFoundException(
        `Unable to retrieve power level for admin in room: ${roomID}`,
        LogContext.MATRIX_ADMIN
      );
    }
    this.logRoomPowerLevelsEvent(powerLevelsEvent);
    return powerLevelsEvent;
  }

  private logRoomPowerLevelsEvent(powerLevelsEvent: IStateEventWithRoomId) {
    const powerLevelsEventUsers = powerLevelsEvent.content.users;
    const poserLevelsEventDefaultUser = powerLevelsEvent.content.users_default;
    this.logger.verbose?.(
      `...room (${
        powerLevelsEvent.room_id
      }) state event: - users: ${JSON.stringify(
        powerLevelsEventUsers
      )} - user_default: ${poserLevelsEventDefaultUser}`,
      LogContext.MATRIX_ADMIN
    );
  }

  private async createMatrixClientForAdmin(adminUser: IOperationalMatrixUser) {
    const adminAgent =
      await this.matrixAgentService.createMatrixAgent(adminUser);

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
        LogContext.MATRIX_ADMIN
      );
    }
    this.logger.verbose?.(
      `User is registered: ${username}, logging in...`,
      LogContext.MATRIX_ADMIN
    );
    const adminUser = await this.matrixUserManagementService.login(
      adminCommunicationsID,
      password
    );
    return adminUser;
  }

  public async updateRoomStateForAdminRooms(
    updateRoomStateForAdminRooms: MatrixAdminEventUpdateRoomStateForAdminRoomsInput
  ) {
    this.logger.verbose?.(
      `*** Updating room state in rooms for admin: ${updateRoomStateForAdminRooms.adminEmail} ***`,
      LogContext.MATRIX_ADMIN
    );
    // See if matches current admin user, if not create a new agent
    let agent =
      await this.communicationAdminUserService.getMatrixAgentElevated();
    const elevatedAgentUser =
      this.communicationAdapter.getUserIdFromMatrixClient(agent.matrixClient);
    const adminUser = await this.getGlobalAdminUser(
      updateRoomStateForAdminRooms.adminEmail,
      updateRoomStateForAdminRooms.adminPassword
    );
    if (elevatedAgentUser !== adminUser.username) {
      agent = await this.createMatrixClientForAdmin(adminUser);
    }
    const matrixClient = agent.matrixClient;
    const adminUserID =
      this.communicationAdapter.getUserIdFromMatrixClient(matrixClient);
    const rooms = matrixClient.getRooms();
    let roomsUpdated = 0;
    try {
      for (const room of rooms) {
        this.logger.verbose?.(
          `Updating room state (power levels) in room: ${room.roomId}`,
          LogContext.MATRIX_ADMIN
        );
        const roomID = room.roomId;

        const powerLevelsEvent = await this.getPowerLevelsEventForRoom(
          matrixClient,
          roomID
        );
        const newPowerLevelDefaultUser =
          updateRoomStateForAdminRooms.powerLevel.users_default;
        const newPowerLevelRedact =
          updateRoomStateForAdminRooms.powerLevel.redact;

        const usersMap = this.convertUsersStringToMap(
          JSON.stringify(powerLevelsEvent.content.users)
        );

        const userPowerLevel = usersMap.get(adminUserID);
        if (userPowerLevel && userPowerLevel === 100) {
          // Update the user's power level (e.g., set them as an admin)
          const updatedPowerLevelsInput: RoomPowerLevelsEventContent = {
            ...powerLevelsEvent.content,
            users_default: newPowerLevelDefaultUser,
            redact: newPowerLevelRedact,
          };

          await matrixClient.sendStateEvent(
            roomID,
            EventType.RoomPowerLevels,
            updatedPowerLevelsInput
          );
          roomsUpdated++;
          this.logger.verbose?.(
            `...===> room state update sent for room ${roomID}`,
            LogContext.MATRIX_ADMIN
          );
        } else {
          if (!userPowerLevel) {
            this.logger.verbose?.(
              `...room state update __skipped__ as current user is not a known user: ${JSON.stringify(
                powerLevelsEvent.content.users
              )}`,
              LogContext.MATRIX_ADMIN
            );
          } else {
            this.logger.verbose?.(
              `...room state update __skipped__ as current user (${adminUserID}) power level is too low: ${userPowerLevel}`,
              LogContext.MATRIX_ADMIN
            );
          }
        }
      }
      this.logger.verbose?.(
        `*** Power levels updated in ${roomsUpdated} rooms for admin: ${updateRoomStateForAdminRooms.adminEmail} ***`,
        LogContext.MATRIX_ADMIN
      );
    } catch (error) {
      const errorMessage = `Unable to update power levels in rooms: ${error}`;
      this.logger.error(errorMessage, LogContext.MATRIX_ADMIN);
      throw error;
    }
  }

  private convertUsersStringToMap(usersString: string): Map<string, number> {
    const usersMap = new Map<string, number>();
    const users = JSON.parse(usersString);
    for (const user in users) {
      usersMap.set(user, users[user]);
    }
    return usersMap;
  }

  public async logRoomState(
    logRoomStateData: MatrixAdminEventLogRoomStateInput
  ) {
    // Note: has side effect of admin user becoming a member of the room if not already one
    const matrixAgentElevated =
      await this.communicationAdminUserService.getMatrixAgentElevated();
    const matrixClient = matrixAgentElevated.matrixClient;
    await this.communicationAdapter.ensureMatrixClientIsMemberOfRoom(
      matrixClient,
      logRoomStateData.roomID
    );
    const roomState = await this.getPowerLevelsEventForRoom(
      matrixClient,
      logRoomStateData.roomID
    );
    this.logger.verbose?.(
      `Room (${logRoomStateData.roomID}:`,
      LogContext.MATRIX_ADMIN
    );
    const usersMap = this.convertUsersStringToMap(
      JSON.stringify(roomState.content.users)
    );
    usersMap.forEach((value, key) => {
      this.logger.verbose?.(
        `...user: ${key} - power level: ${value}`,
        LogContext.MATRIX_ADMIN
      );
    });
    this.logger.verbose?.(
      `...user_default: ${roomState.content.users_default}`,
      LogContext.MATRIX_ADMIN
    );
    this.logger.verbose?.(
      `...redact: ${roomState.content.redact}`,
      LogContext.MATRIX_ADMIN
    );
  }
}

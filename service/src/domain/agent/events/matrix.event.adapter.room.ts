import { LogContext } from '@common/enums/logging.context';
import { LoggerService } from '@nestjs/common';
import { CommunicationEventMessageReceived } from '@src/services/communication-adapter/dto/communication.dto.event.message.received';
import { MatrixRoomInvitationReceived } from '@src/services/communication-adapter/dto/communication.dto.room.invitation.received';
import { KnownMembership,MatrixEvent, RoomMember } from 'matrix-js-sdk';

import { MatrixRoom } from '../../room/matrix.room';
import { MatrixAgent } from '../agent/matrix.agent';
import { IMatrixEventHandler } from './matrix.event.handler.interface';
import { MatrixEventsInternalNames } from './types/matrix.event.internal.names';
import { RoomTimelineEvent } from './types/room.timeline.event';

const noop = function () {
  // empty
};

export const roomMembershipLeaveGuardFactory = (
  targetUserID: string,
  targetRoomID: string
) => {
  return ({ event, member }: { event: MatrixEvent; member: RoomMember }) => {
    const content = event.getContent();
    if (content.membership === KnownMembership.Leave && member.userId === targetUserID) {
      const roomId = event.getRoomId();

      return roomId === targetRoomID;
    }

    return false;
  };
};
export class ForgetRoomMembershipMonitorFactory {
  static create(
    agent: MatrixAgent,
    logger: LoggerService,
    onRoomLeft: () => void,
    onComplete = noop,
    error: (err: Error) => void = noop
  ): IMatrixEventHandler[MatrixEventsInternalNames.RoomMemberMembershipMonitor] {
    return {
      complete: onComplete,
      error: error,
      next: async ({ event, member }) => {
        const content = event.getContent();
        const roomId = event.getRoomId();
        if (roomId) {
          await agent.matrixClient.forget(roomId);
          logger.verbose?.(
            `[Membership] Room [${roomId}] left - user (${member.userId}), membership status ${content.membership}`,
            LogContext.COMMUNICATION
          );
          onRoomLeft();
        }
      },
    };
  }
}

export const autoAcceptRoomGuardFactory = (
  targetUserID: string,
  targetRoomID: string
) => {
  return ({ event, member }: { event: MatrixEvent; member: RoomMember }) => {
    const content = event.getContent();
    if (content.membership === KnownMembership.Invite && member.userId === targetUserID) {
      const roomId = event.getRoomId();

      return roomId === targetRoomID;
    }

    return false;
  };
};
export class AutoAcceptSpecificRoomMembershipMonitorFactory {
  static create(
    agent: MatrixAgent,
    logger: LoggerService,
    targetRoomId: string,
    onRoomJoined: () => void,
    onComplete = noop,
    error: (err: Error) => void = noop
  ): IMatrixEventHandler[MatrixEventsInternalNames.RoomMemberMembershipMonitor] {
    return {
      complete: onComplete,
      error: error,
      next: async ({ event, member }) => {
        const content = event.getContent();
        if (
          content.membership === KnownMembership.Invite &&
          member.userId === agent.matrixClient.credentials.userId
        ) {
          const roomId = event.getRoomId();
          if (!roomId) {
            logger.error?.(
              `[Membership] Unable to accept invitation for user (${member.userId}) to room: ${roomId}`,
              LogContext.COMMUNICATION
            );
            return;
          }

          if (roomId !== targetRoomId) {
            logger.verbose?.(
              `[Membership] skipping invitation for user (${member.userId}) to room: ${roomId}`,
              LogContext.COMMUNICATION
            );
          }

          const senderId = event.getSender();
          if (!senderId) {
            logger.error?.(
              `[Membership] Unable to accept invitation for sender (${senderId}) to room: ${roomId}`,
              LogContext.COMMUNICATION
            );
            return;
          }

          logger.verbose?.(
            `[Membership] accepting invitation for user (${member.userId}) to room: ${roomId}`,
            LogContext.COMMUNICATION
          );
          await agent.matrixClient.joinRoom(roomId);
          if (content.is_direct) {
            await agent.storeDirectMessageRoom(agent, roomId, senderId);
          }
          logger.verbose?.(
            `[Membership] accepted invitation for user (${member.userId}) to room: ${roomId}`,
            LogContext.COMMUNICATION
          );
          onRoomJoined();
        }
      },
    };
  }
}

export class RoomTimelineMonitorFactory {
  static create(
    agent: MatrixAgent,
    logger: LoggerService,
    onMessageReceived: (event: CommunicationEventMessageReceived) => void
  ): IMatrixEventHandler[MatrixEventsInternalNames.RoomTimelineMonitor] {
    return {
      complete: noop,
      error: noop,
      next: async ({ event, room }: RoomTimelineEvent) => {
        logger.verbose?.(
          `RoomTimelineMonitor: received event of type ${
            event.event.type
          } with id ${event.event.event_id} and body: ${
            event.getContent().body
          }`,
          LogContext.COMMUNICATION
        );
        const ignoreMessage = agent.isEventToIgnore(event);

        // TODO Notifications - Allow the client to see the event and then mark it as read
        // With the current behavior the message will automatically be marked as read
        // to ensure that we are returning only the actual updates
        await agent.matrixClient.sendReadReceipt(event);

        if (!ignoreMessage) {
          const message = agent.convertFromMatrixMessage(event);
          logger.verbose?.(
            `Triggering messageReceived event for msg body: ${
              event.getContent().body
            }`,
            LogContext.COMMUNICATION
          );

          const userID = agent.getUserId();

          onMessageReceived({
            message,
            roomId: room.roomId,
            communicationID: userID,
            communityId: undefined,
            roomName: room.name,
          });
        }
      },
    };
  }
}

export class RoomMonitorFactory {
  static create(
    onMessageReceived: (event: MatrixRoomInvitationReceived) => void
  ): IMatrixEventHandler[MatrixEventsInternalNames.RoomMonitor] {
    return {
      complete: noop,
      error: noop,
      // eslint-disable-next-line @typescript-eslint/require-await
      next: async ({ room }: { room: MatrixRoom }) => {
        onMessageReceived({
          roomId: room?.roomId,
        });
      },
    };
  }
}

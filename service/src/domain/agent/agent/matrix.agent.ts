import { LogContext } from '@common/enums/logging.context';
import pkg from '@nestjs/common';
import {
  autoAcceptRoomGuardFactory,
  AutoAcceptSpecificRoomMembershipMonitorFactory,
  ForgetRoomMembershipMonitorFactory,
  roomMembershipLeaveGuardFactory,
  RoomMonitorFactory,
  RoomTimelineMonitorFactory,
} from '@src/domain/agent/events/matrix.event.adapter.room';
import {
  IConditionalMatrixEventHandler,
  IMatrixEventHandler,
  MatrixEventDispatcher,
  InternalEventNames,
} from '@src/domain/agent/events/matrix.event.dispatcher';
import { Disposable } from '@src/common/interfaces/disposable.interface';
import { MatrixClient, Room } from 'matrix-js-sdk';
import { ConfigService } from '@nestjs/config';
import { SlidingWindowManager } from '../sliding-sync/matrix.room.sliding.sync.window.manager';
import { MatrixRoomAdapter } from '@src/domain/adapter-room/matrix.room.adapter';
import { MatrixMessageAdapter } from '@src/domain/adapter-message/matrix.message.adapter';
import { MatrixAgentStartOptions } from './type/matrix.agent.start.options';
import { MatrixRoom } from '@src/domain/room/matrix.room';
import { MatrixEntityNotFoundException } from '@src/common/exceptions/matrix.entity.not.found.exception';

// Wraps an instance of the client sdk
export class MatrixAgent implements Disposable {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
  roomAdapter: MatrixRoomAdapter;
  messageAdapter: MatrixMessageAdapter;
  configService: ConfigService;

  // SLIDING SYNC COMPONENTS
  private slidingWindowManager?: SlidingWindowManager;

  constructor(
    matrixClient: MatrixClient,
    roomAdapter: MatrixRoomAdapter,
    messageAdapter: MatrixMessageAdapter,
    configService: ConfigService,
    private logger: pkg.LoggerService
  ) {
    this.matrixClient = matrixClient;
    this.eventDispatcher = new MatrixEventDispatcher(this.matrixClient);
    this.roomAdapter = roomAdapter;
    this.messageAdapter = messageAdapter;
    this.configService = configService;
  }

  attach(handler: IMatrixEventHandler) {
    this.eventDispatcher.attach(handler);
  }

  attachOnceConditional(handler: IConditionalMatrixEventHandler) {
    this.eventDispatcher.attachOnceConditional(handler);
  }

  detach(id: string) {
    this.eventDispatcher.detach(id);
  }

  async start(
    {
      registerRoomMonitor = true,
      registerTimelineMonitor = false,
    }: MatrixAgentStartOptions = {
      registerRoomMonitor: true,
      registerTimelineMonitor: false,
    }
  ) {
    await this.startWithSlidingSync();

    // Common event handler setup
    const eventHandler: IMatrixEventHandler = {
      id: 'root',
    };

    if (registerTimelineMonitor) {
      eventHandler[InternalEventNames.RoomTimelineMonitor] =
        this.resolveRoomTimelineEventHandler();
    }

    if (registerRoomMonitor) {
      eventHandler[InternalEventNames.RoomMonitor] =
        this.resolveRoomEventHandler();
    }

    this.attach(eventHandler);
  }

  resolveAutoAcceptRoomMembershipMonitor(
    roomId: string,
    onRoomJoined: () => void,
    onComplete?: () => void,
    onError?: (err: Error) => void
  ) {
    return AutoAcceptSpecificRoomMembershipMonitorFactory.create(
      this,
      this.roomAdapter,
      this.logger,
      roomId,
      onRoomJoined,
      onComplete,
      onError
    );
  }

  resolveAutoForgetRoomMembershipMonitor(
    onRoomJoined: () => void,
    onComplete?: () => void,
    onError?: (err: Error) => void
  ) {
    return ForgetRoomMembershipMonitorFactory.create(
      this,
      this.logger,
      onRoomJoined,
      onComplete,
      onError
    );
  }

  resolveAutoAcceptRoomMembershipOneTimeMonitor(
    roomId: string,
    userId: string,
    onRoomJoined: () => void,
    onComplete?: () => void,
    onError?: (err: Error) => void
  ) {
    return {
      observer: this.resolveAutoAcceptRoomMembershipMonitor(
        roomId,
        onRoomJoined,
        onComplete,
        onError
      ),
      condition: autoAcceptRoomGuardFactory(userId, roomId),
    };
  }

  resolveForgetRoomMembershipOneTimeMonitor(
    roomId: string,
    userId: string,
    onRoomLeft: () => void,
    onComplete?: () => void,
    onError?: (err: Error) => void
  ) {
    return {
      observer: this.resolveAutoForgetRoomMembershipMonitor(
        onRoomLeft,
        onComplete,
        onError
      ),
      condition: roomMembershipLeaveGuardFactory(userId, roomId),
    };
  }

  resolveRoomTimelineEventHandler() {
    return RoomTimelineMonitorFactory.create(
      this,
      this.messageAdapter,
      this.logger,
      messageReceivedEvent => {
        this.logger.verbose?.(
          `Room timeline received message: ${messageReceivedEvent.message.message}`,
          LogContext.COMMUNICATION
        );
      }
    );
  }

  resolveRoomEventHandler() {
    return RoomMonitorFactory.create(message => {
      this.logger.verbose?.(
        `Room joined: ${message}`,
        LogContext.COMMUNICATION
      );
    });
  }

  private async startWithSlidingSync(): Promise<void> {
    const config = {
      windowSize: 50,
      sortOrder: 'activity' as const,
      includeEmptyRooms: false,
      ranges: [[0, 49]] as [number, number][],
    };

    this.slidingWindowManager = new SlidingWindowManager(
      this.matrixClient,
      config,
      this.logger
    );

    this.logger.verbose?.(
      'Initializing Sliding Sync with Matrix Client',
      LogContext.MATRIX
    );

    await this.slidingWindowManager.initialize();

    this.logger.verbose?.(
      'Sliding Sync initialized successfully',
      LogContext.MATRIX
    );
  }

  // METHOD FOR ASYNC ROOM ACCESS
  async getRoomAsync(roomId: string): Promise<Room | null> {
    if (this.slidingWindowManager) {
      return await this.slidingWindowManager.getRoomAsync(roomId);
    } else {
      throw new Error(
        'Sliding window manager not initialized. Sliding sync must be enabled.'
      );
    }
  }

  public getUserId(): string {
    const userID = this.matrixClient.getUserId();
    if (!userID) {
      throw new MatrixEntityNotFoundException(
        `Unable to retrieve user on agent: ${this.matrixClient}`,
        LogContext.MATRIX
      );
    }
    return userID;
  }

  public async getRoomOrFail(roomId: string): Promise<MatrixRoom> {
    // SLIDING SYNC APPROACH
    const matrixRoom = await this.getRoomAsync(roomId);

    if (!matrixRoom) {
      throw new MatrixEntityNotFoundException(
        `[User: ${this.getUserId()}] Unable to access Room (${roomId}). Room either does not exist or user does not have access.`,
        LogContext.COMMUNICATION
      );
    }

    return matrixRoom;
  }

  // HELPER METHOD FOR SAFE ROOM OPERATIONS
  async withRoom<T>(
    roomId: string,
    callback: (room: Room) => T | Promise<T>
  ): Promise<T | null> {
    const room = await this.getRoomAsync(roomId);
    if (!room) return null;
    return await callback(room);
  }

  dispose() {
    this.matrixClient.stopClient();
    this.eventDispatcher.dispose.bind(this.eventDispatcher)();

    // Clean up sliding sync resources
    if (this.slidingWindowManager) {
      this.slidingWindowManager.dispose();
    }
  }
}

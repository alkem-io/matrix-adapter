import { LogContext } from '@common/enums/logging.context';
import pkg from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { ConfigurationTypes } from '@src/common/enums/configuration.type';
import { MatrixEntityNotFoundException } from '@src/common/exceptions/matrix.entity.not.found.exception';
import { Disposable } from '@src/common/interfaces/disposable.interface';
import { MatrixMessageAdapter } from '@src/domain/adapter-message/matrix.message.adapter';
import { MatrixRoomAdapter } from '@src/domain/adapter-room/matrix.room.adapter';
import {
  autoAcceptRoomGuardFactory,
  AutoAcceptSpecificRoomMembershipMonitorFactory,
  ForgetRoomMembershipMonitorFactory,
  roomMembershipLeaveGuardFactory,
  RoomMonitorFactory,
  RoomTimelineMonitorFactory,
} from '@src/domain/agent/events/matrix.event.adapter.room';
import {
  MatrixEventDispatcher,
} from '@src/domain/agent/events/matrix.event.dispatcher';
import { MatrixRoom } from '@src/domain/room/matrix.room';
import {
  ISendEventResponse,
  IStartClientOpts,
  MatrixClient,
  TimelineEvents,
} from 'matrix-js-sdk';
import { RoomMessageEventContent } from 'matrix-js-sdk/lib/types';

import { IConditionalMatrixEventHandler } from '../events/matrix.event.conditional.handler.interface';
import { IMatrixEventHandler } from '../events/matrix.event.handler.interface';
import { SlidingWindowManager } from '../sliding-sync/matrix.room.sliding.sync.window.manager';
import { MatrixAgentStartOptions } from './type/matrix.agent.start.options';

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
    this.eventDispatcher = new MatrixEventDispatcher(this);
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
    const startComplete = new Promise<void>((resolve, reject) => {
      const subscription = this.eventDispatcher.syncMonitor.subscribe(
        ({ oldSyncState, syncState }) => {
          if (syncState === 'SYNCING' && oldSyncState !== 'SYNCING') {
            subscription.unsubscribe();
            resolve();
          } else if (syncState === 'ERROR') {
            // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors
            reject();
          }
        }
      );
    });

    const pollTimeout = Number(
      this.configService.get(ConfigurationTypes.MATRIX)?.client
        .startupPollTimeout
    );

    const initialSyncLimit = Number(
      this.configService.get(ConfigurationTypes.MATRIX)?.client
        .startupInitialSyncLimit
    );

    this.logger.verbose?.(
      `starting up with pollTimeout: ${pollTimeout} and initialSyncLimit: ${initialSyncLimit}`,
      LogContext.MATRIX
    );

    const startClientOptions: IStartClientOpts = {
      disablePresence: true,
      initialSyncLimit: initialSyncLimit,
      pollTimeout: pollTimeout,
      lazyLoadMembers: true,
    };

    await this.matrixClient.startClient(startClientOptions);
    await startComplete;

    const eventHandler: IMatrixEventHandler = {
      id: 'root',
    };

    if (registerTimelineMonitor) {
      eventHandler['roomTimelineMonitor'] =
        this.resolveRoomTimelineEventHandler();
    }

    if (registerRoomMonitor) {
      eventHandler['roomMonitor'] = this.resolveRoomEventHandler();
    }

    this.attach(eventHandler);
  }

  // async startAsync(
  //   {
  //     registerRoomMonitor = true,
  //     registerTimelineMonitor = false,
  //   }: MatrixAgentStartOptions = {
  //     registerRoomMonitor: true,
  //     registerTimelineMonitor: false,
  //   }
  // ) {
  //   await this.startWithSlidingSync();

  //   // Common event handler setup
  //   const eventHandler: IMatrixEventHandler = {
  //     id: 'root',
  //   };

  //   if (registerTimelineMonitor) {
  //     eventHandler[MatrixEventsInternalNames.RoomTimelineMonitor] =
  //       this.resolveRoomTimelineEventHandler();
  //   }

  //   if (registerRoomMonitor) {
  //     eventHandler[MatrixEventsInternalNames.RoomMonitor] =
  //       this.resolveRoomEventHandler();
  //   }

  //   this.attach(eventHandler);
  // }

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

  // private async startWithSlidingSync(): Promise<void> {
  //   const config = {
  //     windowSize: 50000,
  //     sortOrder: 'activity' as const,
  //     includeEmptyRooms: false,
  //     ranges: [[0, 49]] as [number, number][],
  //   };

  //   this.slidingWindowManager = new SlidingWindowManager(
  //     this.matrixClient,
  //     config,
  //     this.logger
  //   );

  //   this.logger.verbose?.(
  //     'Initializing Sliding Sync with Matrix Client',
  //     LogContext.MATRIX
  //   );

  //   await this.slidingWindowManager.initialize();

  //   this.logger.verbose?.(
  //     'Sliding Sync initialized successfully',
  //     LogContext.MATRIX
  //   );
  // }

  // METHOD FOR ASYNC ROOM ACCESS
  // async getRoomAsync(roomId: string): Promise<Room | null> {
  //   if (this.slidingWindowManager) {
  //     return await this.slidingWindowManager.getRoomAsync(roomId);
  //   } else {
  //     throw new Error(
  //       'Sliding window manager not initialized. Sliding sync must be enabled.'
  //     );
  //   }
  // }

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

  public async sendEvent(
    roomId: string,
    type: keyof TimelineEvents,
    content: RoomMessageEventContent
  ): Promise<ISendEventResponse> {
    return await this.matrixClient.sendEvent(
      roomId,
      type,
      content
    );
  }

  public  getRoomOrFail(roomId: string): MatrixRoom {
    // SLIDING SYNC APPROACH
    // const matrixRoom = await this.getRoomAsync(roomId);
    const matrixRoom = this.matrixClient.getRoom(roomId);

    if (!matrixRoom) {
      throw new MatrixEntityNotFoundException(
        `[User: ${this.getUserId()}] Unable to access Room (${roomId}). Room either does not exist or user does not have access.`,
        LogContext.COMMUNICATION
      );
    }

    return matrixRoom;
  }

  // // HELPER METHOD FOR SAFE ROOM OPERATIONS
  // async withRoom<T>(
  //   roomId: string,
  //   callback: (room: Room) => T | Promise<T>
  // ): Promise<T | null> {
  //   const room = await this.getRoomAsync(roomId);
  //   if (!room) return null;
  //   return await callback(room);
  // }

  dispose() {
    this.matrixClient.stopClient();
    this.eventDispatcher.dispose.bind(this.eventDispatcher)();

    // Clean up sliding sync resources
    if (this.slidingWindowManager) {
      this.slidingWindowManager.dispose();
    }
  }
}

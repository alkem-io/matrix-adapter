import { IMessage } from '@alkemio/matrix-adapter-lib';
import { LogContext } from '@common/enums/logging.context';
import pkg from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { MatrixEntityNotFoundException } from '@src/common/exceptions/matrix.entity.not.found.exception';
import { Disposable } from '@src/common/interfaces/disposable.interface';
import { MatrixMessageAdapter } from '@src/domain/adapter-message/matrix.message.adapter';
import { MatrixRoomResponseMessage } from '@src/domain/adapter-room/dto/matrix.room.dto.response.message';
import {
  autoAcceptRoomGuardFactory,
  AutoAcceptSpecificRoomMembershipMonitorFactory,
  ForgetRoomMembershipMonitorFactory,
  roomMembershipLeaveGuardFactory,
  RoomMonitorFactory,
  RoomTimelineMonitorFactory,
} from '@src/domain/agent/events/matrix.event.adapter.room';
import { MatrixEventDispatcher } from '@src/domain/agent/events/matrix.event.dispatcher';
import { MatrixRoom } from '@src/domain/room/matrix.room';
import {
  EventType,
  IContent,
  ISendEventResponse,
  MatrixClient,
  TimelineEvents,
} from 'matrix-js-sdk';
import { RoomMessageEventContent } from 'matrix-js-sdk/lib/types';

import { IConditionalMatrixEventHandler } from '../events/matrix.event.conditional.handler.interface';
import { IMatrixEventHandler } from '../events/matrix.event.handler.interface';
import { MatrixEventsInternalNames } from '../events/types/matrix.event.internal.names';
import { SlidingWindowManager } from '../sliding-sync/matrix.room.sliding.sync.window.manager';
import { SlidingSyncConfig } from '../sliding-sync/matrix.sliding.sync.config';
import { MatrixAgentStartOptions } from './type/matrix.agent.start.options';

// Wraps an instance of the client sdk
export class MatrixAgent implements Disposable {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
  messageAdapter: MatrixMessageAdapter;
  configService: ConfigService;

  // SLIDING SYNC COMPONENTS
  private slidingWindowManager?: SlidingWindowManager;

  constructor(
    matrixClient: MatrixClient,
    configService: ConfigService,
    messageAdapter: MatrixMessageAdapter,
    private logger: pkg.LoggerService
  ) {
    this.matrixClient = matrixClient;
    this.eventDispatcher = new MatrixEventDispatcher(this);
    this.configService = configService;
    this.messageAdapter = messageAdapter;
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

  // Using sliding sync for better performance
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
      eventHandler[MatrixEventsInternalNames.RoomTimelineMonitor] =
        this.resolveRoomTimelineEventHandler();
    }

    if (registerRoomMonitor) {
      eventHandler[MatrixEventsInternalNames.RoomMonitor] =
        this.resolveRoomEventHandler();
    }

    this.attach(eventHandler);
  }

  private getSlidingSyncConfiguration(): SlidingSyncConfig {

      //   const pollTimeout = Number(
  //     this.configService.get(ConfigurationTypes.MATRIX)?.client
  //       .startupPollTimeout
  //   );

  //   const initialSyncLimit = Number(
  //     this.configService.get(ConfigurationTypes.MATRIX)?.client
  //       .startupInitialSyncLimit
  //   );

  //   this.logger.verbose?.(
  //     `starting up with pollTimeout: ${pollTimeout} and initialSyncLimit: ${initialSyncLimit}`,
  //     LogContext.MATRIX
  //   );

  const config = {
      windowSize: 50000,
      sortOrder: 'activity' as const,
      includeEmptyRooms: false,
      ranges: [[0, 49]] as [number, number][],
    };
    return config;
  }

  private async startWithSlidingSync(): Promise<void> {
    const config = this.getSlidingSyncConfiguration();

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

  resolveAutoAcceptRoomMembershipMonitor(
    roomId: string,
    onRoomJoined: () => void,
    onComplete?: () => void,
    onError?: (err: Error) => void
  ) {
    return AutoAcceptSpecificRoomMembershipMonitorFactory.create(
      this,
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

  isEventToIgnore(message: MatrixRoomResponseMessage): boolean {
    return this.messageAdapter.isEventToIgnore(message);
  }

  convertFromMatrixMessage(message: MatrixRoomResponseMessage): IMessage {
    return this.messageAdapter.convertFromMatrixMessage(message);
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

  public async sendEvent(
    roomId: string,
    type: keyof TimelineEvents,
    content: RoomMessageEventContent
  ): Promise<ISendEventResponse> {
    return await this.matrixClient.sendEvent(roomId, type, content);
  }

  // there could be more than one dm room per user
  getDirectMessageRoomsMap(): Record<string, string[]> {
    const mDirectEvent = this.matrixClient.getAccountData(EventType.Direct);
    // todo: tidy up this logic
    const eventContent = mDirectEvent
      ? mDirectEvent.getContent<IContent>()
      : {};

    const userId = this.getUserId();

    // there is a bug in the sdk
    const selfDMs = eventContent[userId];
    if (selfDMs && selfDMs.length) {
      // it seems that two users can have multiple DM rooms between them and only one needs to be active
      // they have fixed the issue inside the react-sdk instead of the js-sdk...
    }

    return eventContent;
  }

  async storeDirectMessageRoom(
    agent: MatrixAgent,
    roomId: string,
    userId: string
  ) {
    // NOT OPTIMIZED - needs caching
    const dmRooms = this.getDirectMessageRoomsMap();

    dmRooms[userId] = [roomId];
    await agent.matrixClient.setAccountData(EventType.Direct, dmRooms);
  }

  private getSlidingSyncManager(): SlidingWindowManager {
    if (!this.slidingWindowManager) {
      throw new Error(
        'SlidingWindowManager is not initialized. Please call start() first.'
      );
    }
    return this.slidingWindowManager;
  }

  public async getRoomOrFail(roomId: string): Promise<MatrixRoom> {
    const matrixRoom = await this.getSlidingSyncManager().getRoomAsync(roomId);

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

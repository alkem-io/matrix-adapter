import { IMessage } from '@alkemio/matrix-adapter-lib';
import { LogContext } from '@common/enums/logging.context';
import pkg from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { ConfigurationTypes } from '@src/common/enums/configuration.type';
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
import { SlidingSync } from 'matrix-js-sdk/lib/sliding-sync.js';
import { SlidingSyncSdk } from 'matrix-js-sdk/lib/sliding-sync-sdk.js';
import { SyncApi } from 'matrix-js-sdk/lib/sync.js';
import { RoomMessageEventContent } from 'matrix-js-sdk/lib/types';

import { IConditionalMatrixEventHandler } from '../events/matrix.event.conditional.handler.interface';
import { IMatrixEventHandler } from '../events/matrix.event.handler.interface';
import { MatrixEventsInternalNames } from '../events/types/matrix.event.internal.names';
import { SlidingSyncConfig } from './matrix.sliding.sync.config';
import { MatrixAgentStartOptions } from './type/matrix.agent.start.options';

type CircuitBreakerState = "CLOSED" | "OPEN" | "HALF_OPEN";

class PeekCircuitBreaker {
  private state: CircuitBreakerState = "CLOSED";
  private failureCount = 0;
  private lastFailureTime = 0;
  private readonly failureThreshold = 5;
  private resetTimeout = 250; // Start with 250 milliseconds
  private readonly maxResetTimeout = 30000; // Max 30 seconds

  public async attemptPeek<T>(fn: () => Promise<T>, roomId: string): Promise<T> {
    if (this.state === "OPEN") {
      const timeSinceFailure = Date.now() - this.lastFailureTime;
      if (timeSinceFailure < this.resetTimeout) {
        throw new Error(
          `Circuit breaker is open for room ${roomId}. Retry after ${this.resetTimeout - timeSinceFailure}ms`
        );
      } else {
        // Transition to HALF_OPEN and allow a single test request
        this.state = "HALF_OPEN";
      }
    }

    try {
      const result = await fn();
      // Reset on success
      this.state = "CLOSED";
      this.failureCount = 0;
      this.resetTimeout = 250; // Reset to initial timeout
      return result;
    } catch (error) {
      this.failureCount++;
      this.lastFailureTime = Date.now();

      // Apply exponential backoff
      this.resetTimeout = Math.min(
        this.maxResetTimeout,
        this.resetTimeout * 2
      );

      if (this.failureCount >= this.failureThreshold) {
        this.state = "OPEN";
      }

      throw error;
    }
  }
}

// Wraps an instance of the client sdk
export class MatrixAgent implements Disposable {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
  messageAdapter: MatrixMessageAdapter;
  configService: ConfigService;
  slidingSync?: SlidingSync;
  slidingSyncSdk?: SlidingSyncSdk;
  syncApi?: SyncApi;
  // Initialize the circuit breaker
  private peekCircuitBreaker = new PeekCircuitBreaker();

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
  start(
    {
      registerRoomMonitor = true,
      registerTimelineMonitor = false,
    }: MatrixAgentStartOptions = {
      registerRoomMonitor: true,
      registerTimelineMonitor: false,
    }
  ) {
    this.startWithSlidingSync();

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
    const matrixConfig = this.configService.get(ConfigurationTypes.MATRIX);
    const slidingSyncConfig = matrixConfig?.client?.slidingSync;

    const windowSize = Number(slidingSyncConfig?.windowSize) || 50;
    const sortOrder = slidingSyncConfig?.sortOrder || 'activity';
    const includeEmptyRooms = slidingSyncConfig?.includeEmptyRooms ?? false;

    // Parse ranges properly - it might come as a string from config
    let ranges: [number, number][] = [[0, 99]]; // Default
    if (slidingSyncConfig?.ranges) {
      try {
        if (typeof slidingSyncConfig.ranges === 'string') {
          ranges = JSON.parse(slidingSyncConfig.ranges);
        } else if (Array.isArray(slidingSyncConfig.ranges)) {
          ranges = slidingSyncConfig.ranges;
        }
      } catch (error) {
        this.logger.warn?.(
          `Failed to parse ranges from config, using default: ${error}`,
          LogContext.SLIDING_SYNC
        );
        ranges = [[0, 99]];
      }
    }

    this.logger.verbose?.(
      `Sliding sync config - windowSize: ${windowSize}, sortOrder: ${sortOrder}, includeEmptyRooms: ${includeEmptyRooms}, ranges: ${JSON.stringify(ranges)}`,
      LogContext.SLIDING_SYNC
    );

    const config = {
      windowSize,
      sortOrder: sortOrder as 'activity' | 'alphabetical' | 'unread',
      includeEmptyRooms,
      ranges,
    };
    return config;
  }

  private startWithSlidingSync(): void {
    const config = this.getSlidingSyncConfiguration();

    const lists = new Map<string, any>([
      [
        'allRooms',
        {
          ranges: config.ranges,
          required_state: [
            ['m.room.create', ''],
            ['m.room.name', ''],
            ['m.room.topic', ''],
            ['m.room.member', '$ME'], // Only our own membership
            ['m.room.encryption', ''],
          ],
          timeline_limit: 10,
          sort: ['by_recency'],
          filters: { is_dm: false },
        },
      ],
    ]);

    const roomSubscriptionInfo = {
      timeline_limit: 10,
      required_state: [
        ['*', '*'], // All state events for subscribed rooms
      ],
    };

    this.slidingSync = new SlidingSync(
      this.matrixClient.getHomeserverUrl(),
      lists,
      roomSubscriptionInfo,
      this.matrixClient,
      30000
    );
    this.slidingSyncSdk = new SlidingSyncSdk(
      this.slidingSync,
      this.matrixClient,
      {},
      {}
    );
    this.syncApi = new SyncApi(this.matrixClient, {}, {});

    // Start sliding sync in the background without blocking
    // sync 100 rooms initially then peek for each one requested
    this.slidingSyncSdk.sync().catch(error => {
      this.logger.error?.(
        `Sliding sync failed: ${error}`,
        LogContext.SLIDING_SYNC
      );
    });

    this.logger.verbose?.(
      'Sliding Sync started in background',
      LogContext.SLIDING_SYNC
    );
  }

  public async getRoomOrFail(roomId: string, maxRetries = 5): Promise<MatrixRoom> {
    // First, check if the room is already in the cache
    let matrixRoom = this.matrixClient.getRoom(roomId);
    if (matrixRoom) {
      return matrixRoom;
    }

    // If not, attempt to peek with retry logic
    if (this.syncApi) {
      let lastError: unknown;

      for (let attempt = 1; attempt <= maxRetries; attempt++) {
        try {
          matrixRoom = await this.peekCircuitBreaker.attemptPeek(
            () => this.syncApi!.peek(roomId),
            roomId
          );
          break; // Success - exit retry loop
        } catch (error) {
          lastError = error;
          if (attempt < maxRetries) {
            // Wait with exponential backoff
            const delay = Math.min(1000 * Math.pow(2, attempt), 5000); // Max 5s delay
            this.logger.debug?.(`Peek attempt ${attempt} failed for room ${roomId}. Retrying in ${delay}ms...`);
            await new Promise(resolve => setTimeout(resolve, delay));
          }
        }
      }

      if (!matrixRoom) {
        throw new MatrixEntityNotFoundException(
          `Failed to peek room ${roomId} after ${maxRetries} attempts. Last error: ${lastError}`,
          LogContext.MATRIX
        );
      }
    }

    // Verify the room was loaded after peeking
    matrixRoom = this.matrixClient.getRoom(roomId);
    if (!matrixRoom) {
      throw new MatrixEntityNotFoundException(
        `Room not found after peeking: ${roomId}`,
        LogContext.MATRIX
      );
    }

    return matrixRoom;
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

  dispose() {
    this.matrixClient.stopClient();
    this.eventDispatcher.dispose.bind(this.eventDispatcher)();
  }
}

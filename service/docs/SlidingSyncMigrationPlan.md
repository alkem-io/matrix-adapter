# Sliding Sync Migration Implementation Plan

**Status: COMPLETED - Room Access Layer Removed**

## IMPORTANT UPDATE: Architecture Simplification

The original plan included a RoomAccessLayer abstraction, but this has been **removed** to simplify the architecture. All room access is now handled directly through the SlidingWindowManager. This provides:

- **Simplified codebase** - Direct access without abstraction layers
- **Better performance** - No additional indirection
- **Clearer architecture** - Single point of room access management
- **Easier maintenance** - Fewer components to manage

The migration is now complete with this simplified approach.

---

## Original Migration Plan (For Reference)

## Overview

This document outlines the detailed implementation plan for migrating the matrix-adapter service from Full Sync to Sliding Sync based on code analysis of the current implementation.

## Step 1: Code Audit and Room Access Analysis

### 1.1 Critical Room Access Points Identified

Based on code analysis, the following locations use direct room access patterns that need refactoring:

#### MatrixAgentService.getRoom()
**Location:** `src/services/matrix/agent/matrix.agent.service.ts:111-130`
```typescript
// CURRENT IMPLEMENTATION - BREAKS WITH SLIDING SYNC
async getRoom(matrixAgent: IMatrixAgent, roomId: string): Promise<MatrixRoom> {
  let matrixRoom = matrixAgent.matrixClient.getRoom(roomId); // SYNCHRONOUS ACCESS
  // Retry logic for full sync completion
  const maxRetries = 10;
  let reTries = 0;
  while (!matrixRoom && reTries < maxRetries) {
    await sleep(100);
    matrixRoom = matrixAgent.matrixClient.getRoom(roomId); // ASSUMES COMPLETE STATE
    reTries++;
  }
  if (!matrixRoom) {
    throw new MatrixEntityNotFoundException(
      `Unable to access Room (${roomId}). Room either does not exist or user does not have access.`,
      LogContext.COMMUNICATION
    );
  }
  return matrixRoom;
}
```

#### MatrixRoomAdapter.getMatrixRoom()
**Location:** `src/services/matrix/adapter-room/matrix.room.adapter.ts:221`
```typescript
// CURRENT IMPLEMENTATION
const room = await adminMatrixClient.getRoom(roomID); // DIRECT SYNCHRONOUS ACCESS
```

#### CommunicationAdapter.getAllRooms()
**Location:** `src/services/communication-adapter/communication.adapter.ts:578`
```typescript
// CURRENT IMPLEMENTATION - ASSUMES ALL ROOMS LOADED
const rooms = await elevatedAgent.matrixClient.getRooms(); // GETS ALL ROOMS
```

#### MatrixAdminService Room Access
**Location:** `src/services/matrix-admin/matrix.admin.service.ts:122`
```typescript
// CURRENT IMPLEMENTATION
const rooms = matrixClient.getRooms(); // ASSUMES COMPLETE ROOM LIST
```

### 1.2 MatrixAgent Startup Process Analysis

**Location:** `src/services/matrix/agent/matrix.agent.ts:73-110`

Current initialization uses Full Sync:
```typescript
// CURRENT STARTUP PROCESS
const startClientOptions: IStartClientOpts = {
  disablePresence: true,
  initialSyncLimit: initialSyncLimit,
  pollTimeout: pollTimeout,
  lazyLoadMembers: true,
};

await this.matrixClient.startClient(startClientOptions); // FULL SYNC
await startComplete; // WAITS FOR COMPLETE SYNC
```

### 1.3 Room State Assumptions

The following code patterns assume complete local room state:

1. **Direct member access:** `room.getMembers()` - assumes all members loaded
2. **Room metadata access:** `room.name`, `room.roomId` - immediate access
3. **Timeline access:** `room.getLiveTimeline()` - assumes timeline loaded
4. **Membership checks:** `room.hasMembershipState()` - assumes complete state

## Step 2: Sliding Sync Infrastructure Implementation

### 2.1 SlidingWindowManager Implementation

Create new file: `src/services/matrix/sliding-sync/sliding.window.manager.ts`

```typescript
import { MatrixClient, SlidingSync, ISlidingSyncOptions } from 'matrix-js-sdk';
import { Injectable } from '@nestjs/common';

export interface SlidingWindowConfig {
  windowSize: number;
  sortOrder: 'activity' | 'alphabetical' | 'unread';
  includeEmptyRooms: boolean;
  ranges: [number, number][];
}

@Injectable()
export class SlidingWindowManager {
  private client: MatrixClient;
  private slidingSync: SlidingSync;
  private config: SlidingWindowConfig;
  private activeRooms: Map<string, Room> = new Map();
  private roomCache: Map<string, Room> = new Map();
  private isInitialized = false;

  constructor(client: MatrixClient, config: SlidingWindowConfig) {
    this.client = client;
    this.config = config;
  }

  async initialize(): Promise<void> {
    const slidingSyncOptions: ISlidingSyncOptions = {
      client: this.client,
      ranges: this.config.ranges,
      sort: [this.config.sortOrder],
      extensions: {
        // Configure extensions as needed
      }
    };

    this.slidingSync = new SlidingSync(slidingSyncOptions);

    // Set up event listeners
    this.slidingSync.on('response', this.onSlidingSyncResponse.bind(this));

    await this.slidingSync.start();
    this.isInitialized = true;
  }

  async ensureRoomLoaded(roomId: string): Promise<Room | null> {
    // Check if room is already in our window
    if (this.activeRooms.has(roomId)) {
      return this.activeRooms.get(roomId)!;
    }

    // Check cache first
    if (this.roomCache.has(roomId)) {
      const room = this.roomCache.get(roomId)!;
      this.activeRooms.set(roomId, room);
      return room;
    }

    // Need to request this specific room
    await this.requestSpecificRoom(roomId);

    return this.activeRooms.get(roomId) || null;
  }

  private async requestSpecificRoom(roomId: string): Promise<void> {
    // Implementation depends on sliding sync API for specific room requests
    // This is a complex operation that may require modifying the sliding window
    // or using separate room-specific requests
  }

  private onSlidingSyncResponse(data: any): void {
    // Handle room additions/removals from window
    // Update activeRooms map
    // Manage cache
    // Emit events for UI updates
  }

  async getRoomAsync(roomId: string): Promise<Room | null> {
    return await this.ensureRoomLoaded(roomId);
  }

  isRoomInWindow(roomId: string): boolean {
    return this.activeRooms.has(roomId);
  }

  dispose(): void {
    if (this.slidingSync) {
      this.slidingSync.stop();
    }
  }
}
```

### 2.2 Room Access Abstraction Layer

Create new file: `src/services/matrix/sliding-sync/room.access.layer.ts`

```typescript
export interface IRoomAccessLayer {
  getRoomAsync(roomId: string): Promise<Room | null>;
  ensureRoomLoaded(roomId: string): Promise<void>;
  isRoomInWindow(roomId: string): boolean;
  getRoomList(offset: number, limit: number): Promise<Room[]>;
  getRoomSummary(roomId: string): Promise<RoomSummary>;
  getTotalRoomCount(): Promise<number>;
}

export interface RoomSummary {
  roomId: string;
  name?: string;
  unreadCount: number;
  notificationCount: number;
  lastActivity: Date;
}

@Injectable()
export class RoomAccessLayer implements IRoomAccessLayer {
  constructor(
    private slidingWindowManager: SlidingWindowManager,
    private matrixClient: MatrixClient
  ) {}

  async getRoomAsync(roomId: string): Promise<Room | null> {
    return await this.slidingWindowManager.ensureRoomLoaded(roomId);
  }

  async ensureRoomLoaded(roomId: string): Promise<void> {
    await this.slidingWindowManager.ensureRoomLoaded(roomId);
  }

  isRoomInWindow(roomId: string): boolean {
    return this.slidingWindowManager.isRoomInWindow(roomId);
  }

  async getRoomList(offset: number, limit: number): Promise<Room[]> {
    // Implementation for paginated room access
    // This may require sliding the window or using room summaries
    throw new Error('Not implemented');
  }

  async getRoomSummary(roomId: string): Promise<RoomSummary> {
    // Get room summary without loading full room state
    // This uses sliding sync's summary capabilities
    throw new Error('Not implemented');
  }

  async getTotalRoomCount(): Promise<number> {
    // Get total room count from sliding sync
    throw new Error('Not implemented');
  }
}
```

## Step 3: MatrixAgent Refactoring

### 3.1 Update MatrixAgent Constructor

**File:** `src/services/matrix/agent/matrix.agent.ts`

```typescript
// ADD TO IMPORTS
import { SlidingWindowManager } from '../sliding-sync';

// MODIFY CONSTRUCTOR
export class MatrixAgent implements IMatrixAgent, Disposable {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
  roomAdapter: MatrixRoomAdapter;
  messageAdapter: MatrixMessageAdapter;
  configService: ConfigService;

  // NEW SLIDING SYNC COMPONENTS
  private slidingWindowManager?: SlidingWindowManager;
  private useSlidingSync: boolean;

  constructor(
    matrixClient: MatrixClient,
    roomAdapter: MatrixRoomAdapter,
    messageAdapter: MatrixMessageAdapter,
    configService: ConfigService,
    private logger: pkg.LoggerService,
    useSlidingSync = false // Feature flag for gradual rollout
  ) {
    this.matrixClient = matrixClient;
    this.eventDispatcher = new MatrixEventDispatcher(this.matrixClient);
    this.roomAdapter = roomAdapter;
    this.messageAdapter = messageAdapter;
    this.configService = configService;
    this.useSlidingSync = useSlidingSync;
  }
```

### 3.2 Update Start Method

```typescript
async start(
  {
    registerRoomMonitor = true,
    registerTimelineMonitor = false,
  }: MatrixAgentStartOptions = {
    registerRoomMonitor: true,
    registerTimelineMonitor: false,
  }
) {
  if (this.useSlidingSync) {
    await this.startWithSlidingSync();
  } else {
    await this.startWithFullSync();
  }

  // Common event handler setup
  const eventHandler: IMatrixEventHandler = {
    id: 'root',
  };

  if (registerTimelineMonitor) {
    eventHandler[InternalEventNames.RoomTimelineMonitor] =
      this.resolveRoomTimelineEventHandler();
  }

  if (registerRoomMonitor) {
    eventHandler[InternalEventNames.RoomMonitor] = this.resolveRoomEventHandler();
  }

  this.attach(eventHandler);
}

private async startWithSlidingSync(): Promise<void> {
  const config = {
    windowSize: 50,
    sortOrder: 'activity' as const,
    includeEmptyRooms: false,
    ranges: [[0, 49]] as [number, number][]
  };

  this.slidingWindowManager = new SlidingWindowManager(this.matrixClient, config);

  await this.slidingWindowManager.initialize();
}

private async startWithFullSync(): Promise<void> {
  // EXISTING FULL SYNC IMPLEMENTATION
  const startComplete = new Promise<void>((resolve, reject) => {
    const subscription = this.eventDispatcher.syncMonitor.subscribe(
      ({ oldSyncState, syncState }) => {
        if (syncState === SyncState.Syncing && oldSyncState !== SyncState.Syncing) {
          subscription.unsubscribe();
          resolve();
        } else if (syncState === SyncState.Error) {
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

  const startClientOptions: IStartClientOpts = {
    disablePresence: true,
    initialSyncLimit: initialSyncLimit,
    pollTimeout: pollTimeout,
    lazyLoadMembers: true,
  };

  await this.matrixClient.startClient(startClientOptions);
  await startComplete;
}
```

### 3.3 Add Room Access Helper Methods

```typescript
// NEW METHOD FOR ASYNC ROOM ACCESS
async getRoomAsync(roomId: string): Promise<Room | null> {
  if (this.useSlidingSync) {
    return await this.slidingWindowManager.ensureRoomLoaded(roomId);
  } else {
    // Fallback to traditional sync
    return this.matrixClient.getRoom(roomId);
  }
}

// HELPER METHOD FOR SAFE ROOM OPERATIONS
async withRoom<T>(roomId: string, callback: (room: Room) => T | Promise<T>): Promise<T | null> {
  const room = await this.getRoomAsync(roomId);
  if (!room) return null;
  return await callback(room);
}
```

## Step 4: Update MatrixAgentService

### 4.1 Refactor getRoom Method

**File:** `src/services/matrix/agent/matrix.agent.service.ts`

```typescript
// REPLACE EXISTING getRoom METHOD
async getRoom(
  matrixAgent: IMatrixAgent,
  roomId: string
): Promise<MatrixRoom> {
  // NEW ASYNC APPROACH
  const matrixRoom = await (matrixAgent as MatrixAgent).getRoomAsync(roomId);

  if (!matrixRoom) {
    throw new MatrixEntityNotFoundException(
      `[User: ${matrixAgent.matrixClient.getUserId()}] Unable to access Room (${roomId}). Room either does not exist or user does not have access.`,
      LogContext.COMMUNICATION
    );
  }

  return matrixRoom;
}
```

### 4.2 Update getDirectRooms Method

```typescript
async getDirectRooms(matrixAgent: IMatrixAgent): Promise<MatrixRoom[]> {
  const matrixClient = matrixAgent.matrixClient;
  const rooms: MatrixRoom[] = [];

  // Direct rooms
  const dmRoomMap = this.matrixRoomAdapter.getDirectMessageRoomsMap(matrixClient);

  // UPDATED TO USE ASYNC ROOM ACCESS
  for (const matrixUsername of Object.keys(dmRoomMap)) {
    const roomId = dmRoomMap[matrixUsername][0];
    const room = await this.getRoom(matrixAgent, roomId); // NOW ASYNC

    room.receiverCommunicationsID =
      this.matrixUserAdapter.convertMatrixUsernameToMatrixID(matrixUsername);
    room.isDirect = true;
    rooms.push(room);
  }
  return rooms;
}
```

## Step 5: Update Communication Adapter

### 5.1 Refactor getAllRooms Method

**File:** `src/services/communication-adapter/communication.adapter.ts`

```typescript
async getAllRooms(): Promise<RoomResult[]> {
  const elevatedAgent =
    await this.communicationAdminUserService.getMatrixManagementAgentElevated();

  this.logger.verbose?.(
    `[Admin] Obtaining all rooms on Matrix instance using ${elevatedAgent.matrixClient.getUserId()}`,
    LogContext.COMMUNICATION
  );

  // UPDATED FOR SLIDING SYNC COMPATIBILITY
  const roomResults: RoomResult[] = [];

  if ((elevatedAgent as MatrixAgent).useSlidingSync) {
    // Use paginated room access for sliding sync
    const totalRooms = await (elevatedAgent as MatrixAgent).slidingWindowManager.getTotalRoomCount() || 0;
    const pageSize = 50;

    for (let offset = 0; offset < totalRooms; offset += pageSize) {
      const rooms = await (elevatedAgent as MatrixAgent).slidingWindowManager.getRoomList(offset, pageSize) || [];

      for (const room of rooms) {
        const memberIDs = await this.matrixRoomAdapter.getMatrixRoomMembers(
          elevatedAgent.matrixClient,
          room.roomId
        );

        if (memberIDs.length === 0) continue;
        if (memberIDs.length === 1) {
          if (memberIDs[0] === elevatedAgent.matrixClient.getUserId()) continue;
        }

        const roomResult = new RoomResult();
        roomResult.id = room.roomId;
        roomResult.displayName = room.name;
        roomResults.push(roomResult);
      }
    }
  } else {
    // EXISTING FULL SYNC APPROACH
    const rooms = await elevatedAgent.matrixClient.getRooms();
    // ... existing implementation
  }

  return roomResults;
}
```

### 5.2 Update getCommunityRoom Method

```typescript
async getCommunityRoom(
  roomID: string,
  withState = false
): Promise<RoomResult> {
  const matrixAgentElevated =
    await this.communicationAdminUserService.getMatrixManagementAgentElevated();

  await this.ensureMatrixClientIsMemberOfRoom(
    matrixAgentElevated.matrixClient,
    roomID
  );

  // UPDATED TO USE ASYNC ROOM ACCESS
  const matrixRoom = await this.matrixAgentService.getRoom(
    matrixAgentElevated,
    roomID
  ); // NOW ASYNC

  const result =
    await this.matrixRoomAdapter.convertMatrixRoomToCommunityRoom(
      matrixAgentElevated.matrixClient,
      matrixRoom
    );

  if (withState) {
    result.joinRule = await this.getRoomStateJoinRule(
      roomID,
      matrixAgentElevated
    );
    result.historyVisibility = await this.getRoomStateHistoryVisibility(
      roomID,
      matrixAgentElevated
    );
  }
  return result;
}
```

## Step 6: Update MatrixRoomAdapter

### 6.1 Refactor getMatrixRoom Method

**File:** `src/services/matrix/adapter-room/matrix.room.adapter.ts`

```typescript
async getMatrixRoom(
  adminMatrixClient: MatrixClient,
  roomID: string
): Promise<MatrixRoom> {
  // CHECK IF CLIENT SUPPORTS SLIDING SYNC
  const matrixAgent = this.getMatrixAgentFromClient(adminMatrixClient);

  if (matrixAgent && (matrixAgent as MatrixAgent).useSlidingSync) {
    const room = await (matrixAgent as MatrixAgent).getRoomAsync(roomID);
    if (!room) {
      throw new MatrixEntityNotFoundException(
        `Unable to access Room (${roomID}). Room either does not exist or user does not have access.`,
        LogContext.COMMUNICATION
      );
    }
    return room;
  } else {
    // FALLBACK TO EXISTING IMPLEMENTATION
    const room = adminMatrixClient.getRoom(roomID);
    if (!room) {
      throw new MatrixEntityNotFoundException(
        `Unable to access Room (${roomID}). Room either does not exist or user does not have access.`,
        LogContext.COMMUNICATION
      );
    }
    return room;
  }
}

// HELPER METHOD TO GET MATRIX AGENT FROM CLIENT
private getMatrixAgentFromClient(matrixClient: MatrixClient): MatrixAgent | null {
  // Implementation depends on how we track the association between
  // MatrixClient and MatrixAgent instances
  // This might require adding a registry or client metadata
  return null; // Placeholder
}
```

## Step 7: Configuration and Feature Flags

### 7.1 Add Configuration Options

**File:** `src/config/matrix.config.ts` (or appropriate config file)

```typescript
export interface MatrixSlidingSyncConfig {
  enabled: boolean;
  windowSize: number;
  sortOrder: 'activity' | 'alphabetical' | 'unread';
  includeEmptyRooms: boolean;
  ranges: [number, number][];
  fallbackToFullSync: boolean;
}

export interface MatrixConfig {
  // ... existing config
  slidingSync: MatrixSlidingSyncConfig;
}
```

### 7.2 Environment Variables

Add to environment configuration:
```bash
MATRIX_SLIDING_SYNC_ENABLED=false
MATRIX_SLIDING_SYNC_WINDOW_SIZE=50
MATRIX_SLIDING_SYNC_SORT_ORDER=activity
MATRIX_SLIDING_SYNC_FALLBACK_ENABLED=true
```

## Step 8: Testing Strategy

### 8.1 Unit Tests Updates

Create new test files:
- `sliding.window.manager.spec.ts`
- `room.access.layer.spec.ts`
- Update existing `matrix.agent.spec.ts`
- Update existing `matrix.agent.service.spec.ts`

### 8.2 Integration Tests

- Test with various room counts (10, 100, 1000+ rooms)
- Test room access patterns
- Test window sliding behavior
- Test fallback mechanisms

### 8.3 Performance Tests

- Measure startup time improvements
- Monitor memory usage
- Test concurrent room access
- Validate notification delivery

## Step 9: Deployment Strategy

### 9.1 Feature Flag Rollout

1. **Phase 1:** Deploy with sliding sync disabled (validation)
2. **Phase 2:** Enable for 10% of users
3. **Phase 3:** Gradually increase to 50%
4. **Phase 4:** Full rollout to 100%
5. **Phase 5:** Remove full sync fallback code

### 9.2 Monitoring and Metrics

Add metrics for:
- Sliding sync initialization time
- Room access cache hit/miss rates
- Window sliding frequency
- Memory usage comparisons
- Error rates and fallback usage

## Success Criteria

- [ ] Initial sync time reduced by >80% for users with 100+ rooms
- [ ] Memory usage reduced by >60% for heavy users
- [ ] Zero message/notification loss during transition
- [ ] Smooth room navigation experience maintained
- [ ] Successful handling of 1000+ room scenarios
- [ ] Fallback mechanism works reliably
- [ ] All existing functionality preserved

## Implementation Timeline

| Phase | Duration | Tasks |
|-------|----------|-------|
| 1 | 1 week | Infrastructure setup, SlidingWindowManager |
| 2 | 1 week | RoomAccessLayer, MatrixAgent updates |
| 3 | 1 week | Service layer updates, configuration |
| 4 | 1 week | Testing, bug fixes, performance optimization |
| 5 | 1 week | Documentation, deployment preparation |
| 6 | 2 weeks | Gradual rollout and monitoring |

**Total Estimated Duration:** 7-8 weeks

## Risk Mitigation

### High Priority Risks
1. **Data Loss:** Comprehensive event logging and fallback mechanisms
2. **Performance Issues:** Extensive testing with realistic data sets
3. **Integration Problems:** Thorough compatibility testing

### Mitigation Strategies
- Maintain full sync fallback throughout rollout
- Implement comprehensive monitoring and alerting
- Use feature flags for immediate rollback capability
- Staged rollout with careful monitoring at each phase

## Notes

- This migration represents a fundamental architectural change
- Backward compatibility is maintained through feature flags
- Performance benefits will be most noticeable for users with many rooms
- The migration can be done gradually with minimal service disruption

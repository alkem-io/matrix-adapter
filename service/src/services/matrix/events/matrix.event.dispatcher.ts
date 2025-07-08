import { Disposable } from '@src/common/interfaces/disposable.interface';
import { EventEmitter } from 'events';
import {
  ClientEvent,
  EmittedEvents,
  MatrixClient,
  MatrixEvent,
  RoomEvent,
  RoomMemberEvent,
  RoomMember,
  SyncState,
  Room,
} from 'matrix-js-sdk';
import { first, fromEvent, Observable, Observer, Subscription } from 'rxjs';
import { MatrixRoom } from '../adapter-room/matrix.room';
import { MatrixEventHandler } from '../types/matrix.event.handler.type';
export interface IMatrixEventDispatcher {
  syncMonitor?: Observable<{ syncState: string; oldSyncState: string }>;
  roomMonitor?: Observable<{ room: MatrixRoom }>;
  roomTimelineMonitor?: Observable<RoomTimelineEvent>;
  roomMemberMembershipMonitor?: Observable<{ event: MatrixEvent; member: RoomMember }>;
}

export interface IMatrixEventHandler {
  id: string;
  syncMonitor?: Observer<{ syncState: string; oldSyncState: string }>;
  roomMonitor?: Observer<{ room: MatrixRoom }>;
  roomTimelineMonitor?: Observer<RoomTimelineEvent>;
  roomMemberMembershipMonitor?: Observer<{ event: MatrixEvent; member: RoomMember }>;
}

export interface IConditionalMatrixEventHandler {
  id: string;
  roomMemberMembershipMonitor?: {
    observer?: Observer<{ event: MatrixEvent; member: RoomMember }>;
    condition: (value: { event: MatrixEvent; member: RoomMember }) => boolean;
  };
}

export type RoomTimelineEvent = {
  event: MatrixEvent;
  room: MatrixRoom;
  toStartOfTimeline: boolean;
  removed: boolean;
};

export class MatrixEventDispatcher
  implements Disposable, IMatrixEventDispatcher
{
  private _emmiter = new EventEmitter();
  private _disposables: (() => void)[] = [];
  private _subscriptions: Record<string, Subscription[]> = {};

  syncMonitor!: Observable<{ syncState: string; oldSyncState: string }>;
  roomMonitor!: Observable<{ room: MatrixRoom }>;
  roomTimelineMonitor!: Observable<RoomTimelineEvent>;
  roomMemberMembershipMonitor!: Observable<{ event: MatrixEvent; member: RoomMember }>;

  constructor(private _client: MatrixClient) {
    this.init();
  }

  private init() {
    this.initMonitor<{ syncState: string; oldSyncState: string }>(
      ClientEvent.Sync,
      'syncMonitor',
      this._syncMonitor
    );
    this.initMonitor<{ room: MatrixRoom }>(ClientEvent.Room, 'roomMonitor', this._roomMonitor);

    this.initMonitor<RoomTimelineEvent>(
      RoomEvent.Timeline,
      'roomTimelineMonitor',
      this._roomTimelineMonitor
    );

    this.initMonitor<{ event: MatrixEvent; member: RoomMember }>(
      RoomMemberEvent.Membership,
      'roomMemberMembershipMonitor',
      this._roomMemberMembershipMonitor
    );
  }

  private initMonitor<T>(
    event: EmittedEvents,
    handler: keyof IMatrixEventDispatcher,
    monitor: MatrixEventHandler
  ) {
    monitor = monitor.bind(this);
    this._client.on(event, monitor);

    // Type assertion is necessary here due to dynamic property assignment
    // The handler key determines which specific Observable type this will be
    (this as any)[handler] = fromEvent<T>(this._emmiter, handler);
    this._disposables.push(() => this._client.off(event, monitor));
  }

  private _syncMonitor(syncState: SyncState, oldSyncState: SyncState | null) {
    this._emmiter.emit('syncMonitor', {
      syncState: syncState.toString(),
      oldSyncState: oldSyncState?.toString() || ''
    });
  }

  private _roomMonitor(room: Room) {
    this._emmiter.emit('roomMonitor', { room: room as MatrixRoom });
  }

  private _roomTimelineMonitor(
    event: MatrixEvent,
    room: Room | undefined,
    toStartOfTimeline: boolean | undefined,
    removed: boolean
  ) {
    if (room) {
      this._emmiter.emit('roomTimelineMonitor', {
        event,
        room: room as MatrixRoom,
        toStartOfTimeline: toStartOfTimeline || false,
        removed,
      });
    }
  }

  private _roomMemberMembershipMonitor(event: MatrixEvent, member: RoomMember) {
    this._emmiter.emit('roomMemberMembershipMonitor', { event, member });
  }

  attach(handler: IMatrixEventHandler) {
    this.detach(handler.id);

    const subscriptions = [];
    if (handler.syncMonitor) {
      subscriptions.push(this.syncMonitor.subscribe(handler.syncMonitor));
    }
    if (handler.roomMonitor) {
      subscriptions.push(this.roomMonitor.subscribe(handler.roomMonitor));
    }
    if (handler.roomTimelineMonitor) {
      subscriptions.push(
        this.roomTimelineMonitor.subscribe(handler.roomTimelineMonitor)
      );
    }
    if (handler.roomMemberMembershipMonitor) {
      subscriptions.push(
        this.roomMemberMembershipMonitor.subscribe(
          handler.roomMemberMembershipMonitor
        )
      );
    }

    this._subscriptions[handler.id] = subscriptions;
  }

  attachOnceConditional(handler: IConditionalMatrixEventHandler) {
    this.detach(handler.id);

    const subscriptions = [];
    if (handler.roomMemberMembershipMonitor) {
      subscriptions.push(
        this.roomMemberMembershipMonitor
          // will only fire once when the condition is met
          .pipe(first(handler.roomMemberMembershipMonitor.condition))
          .subscribe(handler.roomMemberMembershipMonitor.observer)
      );
    }
  }

  detach(id: string) {
    const subscriptions = this._subscriptions[id];
    subscriptions?.forEach(s => s.unsubscribe());

    delete this._subscriptions[id];
  }

  dispose(): void {
    Object.keys(this._subscriptions).forEach(this.detach.bind(this));
    this._disposables.forEach(d => d());
  }
}

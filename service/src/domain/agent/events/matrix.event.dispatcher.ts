import { EventEmitter } from 'events';

import { Disposable } from '@src/common/interfaces/disposable.interface';
import {
  ClientEvent,
  EmittedEvents,
  MatrixEvent,
  Room,
  RoomEvent,
  RoomMember,
  RoomMemberEvent,
  SyncState,
} from 'matrix-js-sdk';
import { first, fromEvent, Observable, Subscription } from 'rxjs';

import { MatrixRoom } from '../../room/matrix.room';
import { MatrixAgent } from '../agent/matrix.agent';
import { IConditionalMatrixEventHandler } from './matrix.event.conditional.handler.interface';
import { IMatrixEventDispatcher } from './matrix.event.dispatcher.interface';
import { IMatrixEventHandler } from './matrix.event.handler.interface';
import { MatrixEventHandler } from './types/matrix.event.handler.type';
import { MatrixEventsInternalNames } from './types/matrix.event.internal.names';
import { RoomTimelineEvent } from './types/room.timeline.event';

export class MatrixEventDispatcher
  implements Disposable, IMatrixEventDispatcher
{
  private _emitter = new EventEmitter();
  private _agent: MatrixAgent;
  private _disposables: (() => void)[] = [];
  private _subscriptions: Record<string, Subscription[]> = {};

  syncMonitor!: Observable<{ syncState: string; oldSyncState: string }>;
  roomMonitor!: Observable<{ room: MatrixRoom }>;
  roomTimelineMonitor!: Observable<RoomTimelineEvent>;
  roomMemberMembershipMonitor!: Observable<{ event: MatrixEvent; member: RoomMember }>;

  constructor(agent: MatrixAgent) {
    this._agent = agent;
    this.init();
  }

  private init() {
    this.initMonitor<{ syncState: string; oldSyncState: string }>(
      ClientEvent.Sync,
      MatrixEventsInternalNames.SyncMonitor,
      this._syncMonitor
    );
    this.initMonitor<{ room: MatrixRoom }>(ClientEvent.Room, MatrixEventsInternalNames.RoomMonitor, this._roomMonitor);

    this.initMonitor<RoomTimelineEvent>(
      RoomEvent.Timeline,
      MatrixEventsInternalNames.RoomTimelineMonitor,
      this._roomTimelineMonitor
    );

    this.initMonitor<{ event: MatrixEvent; member: RoomMember }>(
      RoomMemberEvent.Membership,
      MatrixEventsInternalNames.RoomMemberMembershipMonitor,
      this._roomMemberMembershipMonitor
    );
  }

  private initMonitor<T>(
    event: EmittedEvents,
    handler: keyof IMatrixEventDispatcher,
    monitor: MatrixEventHandler
  ) {
    monitor = monitor.bind(this);
    this._agent.matrixClient.on(event, monitor);

    // Type assertion is necessary here due to dynamic property assignment
    // The handler key determines which specific Observable type this will be
    (this as any)[handler] = fromEvent<T>(this._emitter, handler);
    this._disposables.push(() => this._agent.matrixClient.off(event, monitor));
  }

  private _syncMonitor(syncState: SyncState, oldSyncState: SyncState | null) {
    this._emitter.emit(MatrixEventsInternalNames.SyncMonitor, {
      syncState: syncState.toString(),
      oldSyncState: oldSyncState?.toString() || ''
    });
  }

  private _roomMonitor(room: Room) {
    this._emitter.emit(MatrixEventsInternalNames.RoomMonitor, { room: room as MatrixRoom });
  }

  private _roomTimelineMonitor(
    event: MatrixEvent,
    room: Room | undefined,
    toStartOfTimeline: boolean | undefined,
    removed: boolean
  ) {
    if (room) {
      this._emitter.emit(MatrixEventsInternalNames.RoomTimelineMonitor, {
        event,
        room: room as MatrixRoom,
        toStartOfTimeline: toStartOfTimeline || false,
        removed,
      });
    }
  }

  private _roomMemberMembershipMonitor(event: MatrixEvent, member: RoomMember) {
    this._emitter.emit(MatrixEventsInternalNames.RoomMemberMembershipMonitor, { event, member });
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

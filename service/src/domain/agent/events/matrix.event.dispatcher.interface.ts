import { MatrixEvent, RoomMember } from 'matrix-js-sdk';
import { Observable } from 'rxjs';

import { MatrixRoom } from '../../room/matrix.room';
import { RoomTimelineEvent } from './types/room.timeline.event';

export interface IMatrixEventDispatcher {
  syncMonitor?: Observable<{ syncState: string; oldSyncState: string }>;
  roomMonitor?: Observable<{ room: MatrixRoom }>;
  roomTimelineMonitor?: Observable<RoomTimelineEvent>;
  roomMemberMembershipMonitor?: Observable<{ event: MatrixEvent; member: RoomMember }>;
}

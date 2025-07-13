import { Observer } from 'rxjs';
import { MatrixEvent, RoomMember } from 'matrix-js-sdk';
import { MatrixRoom } from '../../room/matrix.room';
import { RoomTimelineEvent } from './types/room.timeline.event';

export interface IMatrixEventHandler {
  id: string;
  syncMonitor?: Observer<{ syncState: string; oldSyncState: string }>;
  roomMonitor?: Observer<{ room: MatrixRoom }>;
  roomTimelineMonitor?: Observer<RoomTimelineEvent>;
  roomMemberMembershipMonitor?: Observer<{ event: MatrixEvent; member: RoomMember }>;
}

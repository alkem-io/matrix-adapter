import { MatrixEvent } from 'matrix-js-sdk';
import { MatrixRoom } from '../../../room/matrix.room';

export type RoomTimelineEvent = {
  event: MatrixEvent;
  room: MatrixRoom;
  toStartOfTimeline: boolean;
  removed: boolean;
};

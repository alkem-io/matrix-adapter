import { MatrixEvent, RoomMember } from 'matrix-js-sdk';
import { Observer } from 'rxjs';

export interface IConditionalMatrixEventHandler {
  id: string;
  roomMemberMembershipMonitor?: {
    observer?: Observer<{ event: MatrixEvent; member: RoomMember }>;
    condition: (value: { event: MatrixEvent; member: RoomMember }) => boolean;
  };
}

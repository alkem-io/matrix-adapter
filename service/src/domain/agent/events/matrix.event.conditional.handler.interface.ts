import { Observer } from 'rxjs';
import { MatrixEvent, RoomMember } from 'matrix-js-sdk';

export interface IConditionalMatrixEventHandler {
  id: string;
  roomMemberMembershipMonitor?: {
    observer?: Observer<{ event: MatrixEvent; member: RoomMember }>;
    condition: (value: { event: MatrixEvent; member: RoomMember }) => boolean;
  };
}

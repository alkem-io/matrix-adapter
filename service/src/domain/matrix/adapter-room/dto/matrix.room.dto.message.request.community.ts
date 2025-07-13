import { MatrixRoomMessageRequest } from './matrix.room.dto.message.request';

export class MatrixRoomMessageRequestCommunity extends MatrixRoomMessageRequest {
  communityId!: string;
}

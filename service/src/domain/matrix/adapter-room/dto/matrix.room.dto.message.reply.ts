import { MatrixRoomMessageRequest } from './matrix.room.dto.message.request';

export class MatrixRoomMessageReply extends MatrixRoomMessageRequest {
  threadID!: string;
}

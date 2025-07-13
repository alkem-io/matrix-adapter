import { MatrixRoomMessageRequest } from './matrix.room.dto.message.request';

export class MatrixRoomMessageRequestDirect extends MatrixRoomMessageRequest {
  matrixID!: string;
}

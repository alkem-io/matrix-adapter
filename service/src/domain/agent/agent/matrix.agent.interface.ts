import { MatrixEventDispatcher } from '@src/domain/agent/events/matrix.event.dispatcher';
import { MatrixRoom } from '@src/domain/room/matrix.room';
import { MatrixClient } from 'matrix-js-sdk';

export interface IMatrixAgent {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
  getRoomOrFail(roomId: string): Promise<MatrixRoom>;
}

import { MatrixEventDispatcher } from '@src/domain/agent/events/matrix.event.dispatcher';
import { MatrixClient } from 'matrix-js-sdk';

export interface IMatrixAgent {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
}

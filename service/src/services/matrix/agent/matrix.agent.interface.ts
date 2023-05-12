import { MatrixClient } from 'matrix-js-sdk';
import { MatrixEventDispatcher } from '../events/matrix.event.dispatcher';

export interface IMatrixAgent {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
}

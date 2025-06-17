import { MatrixClient } from 'matrix-js-sdk';
import { MatrixEventDispatcher } from '../events/matrix.event.dispatcher.js';

export interface IMatrixAgent {
  matrixClient: MatrixClient;
  eventDispatcher: MatrixEventDispatcher;
}

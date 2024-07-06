import { Room } from 'matrix-js-sdk';

export class MatrixRoom extends Room {
  receiverCommunicationsID? = '';
  isDirect? = false;
}

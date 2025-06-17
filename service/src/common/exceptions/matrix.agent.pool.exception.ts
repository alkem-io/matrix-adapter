import { LogContext, AlkemioErrorStatus } from '@common/enums/index.js';
import { BaseException } from './base.exception.js';

export class MatrixAgentPoolException extends BaseException {
  constructor(error: string, context: LogContext, code?: AlkemioErrorStatus) {
    super(error, context, code ?? AlkemioErrorStatus.MATRIX_AGENT_POOL_ERROR);
  }
}

import { AlkemioErrorStatus,LogContext } from '@common/enums/index';

import { BaseException } from './base.exception';

export class MatrixAgentPoolException extends BaseException {
  constructor(error: string, context: LogContext, code?: AlkemioErrorStatus) {
    super(error, context, code ?? AlkemioErrorStatus.MATRIX_AGENT_POOL_ERROR);
  }
}

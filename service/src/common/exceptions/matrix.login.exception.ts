import { AlkemioErrorStatus,LogContext } from '@common/enums/index';

import { BaseException } from './base.exception';

export class MatrixUserLoginException extends BaseException {
  constructor(error: string, context: LogContext, code?: AlkemioErrorStatus) {
    super(error, context, code ?? AlkemioErrorStatus.MATRIX_LOGIN_FAILED);
  }
}

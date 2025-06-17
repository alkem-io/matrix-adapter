import { LogContext, AlkemioErrorStatus } from '@common/enums/index.js';
import { BaseException } from './base.exception.js';

export class MatrixUserLoginException extends BaseException {
  constructor(error: string, context: LogContext, code?: AlkemioErrorStatus) {
    super(error, context, code ?? AlkemioErrorStatus.MATRIX_LOGIN_FAILED);
  }
}

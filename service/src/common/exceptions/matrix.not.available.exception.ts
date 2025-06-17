import { LogContext, AlkemioErrorStatus } from '@common/enums/index.js';
import { BaseException } from './base.exception.js';

export class MatrixNotAvailableException extends BaseException {
  constructor(error: string, context = LogContext.MATRIX) {
    super(error, context, AlkemioErrorStatus.MATRIX_NOT_AVAILABLE);
  }
}

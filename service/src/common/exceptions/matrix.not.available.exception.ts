import { LogContext, AlkemioErrorStatus } from '@common/enums';
import { BaseException } from './base.exception';

export class MatrixNotAvailableException extends BaseException {
  constructor(error: string, context = LogContext.MATRIX) {
    super(error, context, AlkemioErrorStatus.MATRIX_NOT_AVAILABLE);
  }
}

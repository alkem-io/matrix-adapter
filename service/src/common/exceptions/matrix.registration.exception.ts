import { LogContext } from '@common/enums/index.js';
import { AlkemioErrorStatus } from '@common/enums/alkemio.error.status.js';
import { BaseException } from './base.exception.js';

export class MatrixUserRegistrationException extends BaseException {
  constructor(error: string, context: LogContext, code?: AlkemioErrorStatus) {
    super(
      error,
      context,
      code ?? AlkemioErrorStatus.MATRIX_REGISTRATION_FAILED
    );
  }
}

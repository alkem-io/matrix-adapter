import { AlkemioErrorStatus } from '@common/enums/alkemio.error.status';
import { LogContext } from '@common/enums/index';

import { BaseException } from './base.exception';

export class MatrixUserRegistrationException extends BaseException {
  constructor(error: string, context: LogContext, code?: AlkemioErrorStatus) {
    super(
      error,
      context,
      code ?? AlkemioErrorStatus.MATRIX_REGISTRATION_FAILED
    );
  }
}

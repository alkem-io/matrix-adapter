import { AlkemioErrorStatus, LogContext } from '@common/enums/index.js';
import { BaseException } from './base.exception.js';

export class BootstrapException extends BaseException {
  constructor(error: string) {
    super(error, LogContext.BOOTSTRAP, AlkemioErrorStatus.BOOTSTRAP_FAILED);
  }
}

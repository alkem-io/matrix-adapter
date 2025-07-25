import { AlkemioErrorStatus, LogContext } from '@common/enums/index';

import { BaseException } from './base.exception';

export class BootstrapException extends BaseException {
  constructor(error: string) {
    super(error, LogContext.BOOTSTRAP, AlkemioErrorStatus.BOOTSTRAP_FAILED);
  }
}

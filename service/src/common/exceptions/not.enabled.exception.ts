import { LogContext } from '@common/enums/index.js';
import { AlkemioErrorStatus } from '../enums/alkemio.error.status.js';
import { BaseException } from './base.exception.js';

export class NotEnabledException extends BaseException {
  constructor(error: string, context: LogContext) {
    super(error, context, AlkemioErrorStatus.NOT_ENABLED);
  }
}

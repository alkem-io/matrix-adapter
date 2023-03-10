import { LogContext, AlkemioErrorStatus } from '../enums';
import { BaseException } from './base.exception';

export class NotSupportedException extends BaseException {
  constructor(error: string, context: LogContext) {
    super(error, context, AlkemioErrorStatus.NOT_SUPPORTED);
  }
}

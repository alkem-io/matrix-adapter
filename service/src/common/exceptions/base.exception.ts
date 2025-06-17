import { LogContext, AlkemioErrorStatus } from '@common/enums/index.js';

export class BaseException extends Error {
  private readonly context: LogContext;
  private readonly code: AlkemioErrorStatus | undefined;

  constructor(error: string, context: LogContext, code?: AlkemioErrorStatus) {
    super(error);
    this.code = code;
    this.context = context;
  }

  public getContext(): LogContext {
    return this.context;
  }

  public getCode(): AlkemioErrorStatus | undefined {
    return this.code;
  }
}

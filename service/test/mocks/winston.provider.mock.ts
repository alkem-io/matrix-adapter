import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { LoggerService, ValueProvider } from '@nestjs/common';
import { PublicPart } from '@test/utils';
import { fn } from 'jest-mock';

export const MockWinstonProvider: ValueProvider<PublicPart<LoggerService>> = {
  provide: WINSTON_MODULE_NEST_PROVIDER,
  useValue: {
    error: fn(),
    warn: fn(),
    verbose: fn(),
  },
};

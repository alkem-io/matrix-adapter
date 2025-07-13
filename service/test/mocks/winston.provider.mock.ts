import { LoggerService, ValueProvider } from '@nestjs/common';
import { PublicPart } from '@test/utils';
import { WINSTON_MODULE_NEST_PROVIDER } from 'nest-winston';
import { vi } from 'vitest';

export const MockWinstonProvider: ValueProvider<PublicPart<LoggerService>> = {
  provide: WINSTON_MODULE_NEST_PROVIDER,
  useValue: {
    error: vi.fn(),
    warn: vi.fn(),
    verbose: vi.fn(),
  },
};

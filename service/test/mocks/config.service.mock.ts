import { ValueProvider } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import { PublicPart } from '@test/utils';
import { vi } from 'vitest';

export const MockConfigServiceProvider: ValueProvider<
  PublicPart<ConfigService>
> = {
  provide: ConfigService,
  useValue: {
    get: vi.fn(),
  },
};

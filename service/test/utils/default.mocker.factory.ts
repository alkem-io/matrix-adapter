import { InjectionToken } from '@nestjs/common';
import { vi } from 'vitest';

// Simple Vitest-based mocker for NestJS providers
export const defaultMockerFactory = (token: InjectionToken | undefined) => {
  if (typeof token === 'function') {
    return vi.fn();
  }
  return undefined;
};

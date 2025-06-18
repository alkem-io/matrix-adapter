import { InjectionToken } from '@nestjs/common';
import { ModuleMocker } from 'jest-mock';

const moduleMocker = new ModuleMocker(global);

export const defaultMockerFactory = (token: InjectionToken | undefined) => {
  if (typeof token === 'function') {
    const mockMetadata = moduleMocker.getMetadata(token);
    if (mockMetadata) {
      const Mock = moduleMocker.generateFromMetadata(mockMetadata);
      return new (Mock as any)();
    }
  }
};

import type { Matchers } from '@jest/expect';

type toThrowParameters = Parameters<Matchers<void, any>['toThrow']>[0];

// Utility to test async functions that should throw
export const asyncToThrow = async (
  actual: () => Promise<unknown>,
  error?: string | RegExp | Error | (new (...args: any[]) => Error)
) => await expect(actual).rejects.toThrow(error);

// Utility to test async functions that should throw (Vitest compatible)
export const asyncToThrow = async (
  actual: () => Promise<unknown>,
  error?: string | RegExp | Error | (new (...args: any[]) => Error)
) => await expect(actual).rejects.toThrow(error);

import baseConfig from './jest.config.mjs';

export default {
  ...baseConfig,
  testMatch: ['**/?(*.)+(spec).ts'],
  coverageDirectory: '<rootDir>/coverage-ci',
};

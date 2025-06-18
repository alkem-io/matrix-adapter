import baseConfig from './jest.config.js';

export default {
  ...baseConfig,
  testMatch: ['**/?(*.)+(spec).ts'],
  coverageDirectory: '<rootDir>/coverage-ci',
};

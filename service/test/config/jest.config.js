module.exports = {
  moduleNameMapper: {
    '@common/(.*)': ['<rootDir>/src/common/$1'],
    '@core/(.*)': ['<rootDir>/src/core/$1'],
    '@config/(.*)': ['<rootDir>/src/config/$1'],
    '@src/(.*)': ['<rootDir>/src/$1'],
    '@test/(.*)': ['<rootDir>/test/$1'],
    '@services/(.*)': ['<rootDir>/src/services/$1'],
  },
  moduleFileExtensions: ['js', 'json', 'ts'],
  rootDir: '../../',
  roots: ['<rootDir>/test', '<rootDir>/src'],
  testMatch: ['**/?(*.)+(spec).ts'],
  transform: {
    '^.+\\.(t|j)s$': 'ts-jest',
  },
  coverageDirectory: '<rootDir>/coverage',
  testEnvironment: 'node',
  collectCoverageFrom: [
    '<rootDir>/src/**/*.service.ts',
    '<rootDir>/src/**/utils.ts',
    '<rootDir>/src/**/*.controller.ts',
    '<rootDir>/src/**/*.adapter.ts',
    '<rootDir>/src/**/*.mapper.ts',
    '<rootDir>/src/**/*.builder.ts',
  ],
  testTimeout: 90000,
  collectCoverage: true,
};

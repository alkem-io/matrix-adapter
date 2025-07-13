import js from '@eslint/js';
import typescript from '@typescript-eslint/eslint-plugin';
import typescriptParser from '@typescript-eslint/parser';
import prettier from 'eslint-plugin-prettier';
import simpleImportSort from 'eslint-plugin-simple-import-sort';
import unicorn from 'eslint-plugin-unicorn';
import vitest from 'eslint-plugin-vitest';

export default [
  // Ignore patterns - must be first
  {
    ignores: [
      'node_modules/**',
      'dist/**',
      'build/**',
      'coverage/**',
      'coverage-ci/**',
      '*.d.ts',
      '.next/**',
      '.nuxt/**',
      '.vscode/**',
      '.idea/**',
      '.husky/**',
      '**/*.config.mjs',
      '**/*.config.js',
      'prettier.config.mjs',
      'lint-staged.config.mjs',
      'loader.mjs',
      'message_samples/**',
    ],
  },

  // Base configuration for all files
  {
    languageOptions: {
      globals: {
        console: 'readonly',
        process: 'readonly',
        Buffer: 'readonly',
        __dirname: 'readonly',
        __filename: 'readonly',
        global: 'readonly',
        module: 'readonly',
        require: 'readonly',
        exports: 'readonly',
        NodeJS: 'readonly',
        setInterval: 'readonly',
        clearInterval: 'readonly',
        setTimeout: 'readonly',
        clearTimeout: 'readonly',
      },
    },
    plugins: {
      'simple-import-sort': simpleImportSort,
      prettier,
      unicorn,
    },
    rules: {
      ...js.configs.recommended.rules,

      // Custom rules
      'simple-import-sort/imports': 'error',
      'simple-import-sort/exports': 'error',
      'unicorn/prefer-module': 'off',
      'unicorn/prefer-top-level-await': 'off',
      'unicorn/prevent-abbreviations': 'off',
      'no-console': 'warn',
      'no-process-exit': 'off',
    },
  },

  // TypeScript-specific configuration
  {
    files: ['**/*.ts', '**/*.tsx'],
    languageOptions: {
      parser: typescriptParser,
      parserOptions: {
        project: './tsconfig.json',
      },
    },
    plugins: {
      '@typescript-eslint': typescript,
    },
    rules: {
      ...typescript.configs.recommended.rules,
      ...typescript.configs['recommended-type-checked'].rules,
      
      // Disable unsafe type checking rules for legacy code
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/restrict-template-expressions': 'off',
      '@typescript-eslint/no-base-to-string': 'off',
      '@typescript-eslint/no-unsafe-enum-comparison': 'off',
      '@typescript-eslint/no-misused-promises': 'off',
      '@typescript-eslint/unbound-method': 'off',
      'simple-import-sort/imports': [
        'error',
        {
          groups: [
            [
              '^(assert|buffer|child_process|cluster|console|constants|crypto|dgram|dns|domain|events|fs|http|https|module|net|os|path|punycode|querystring|readline|repl|stream|string_decoder|sys|timers|tls|tty|url|util|vm|zlib|freelist|v8|process|async_hooks|http2|perf_hooks)(/.*|$)',
            ],
            [
              '^node:.*\\u0000$',
              '^@?\\w.*\\u0000$',
              '^[^.].*\\u0000$',
              '^\\..*\\u0000$',
            ],
            ['^\\u0000'],
            ['^node:'],
            ['^@?\\w'],
            ['^@/tests(/.*|$)'],
            ['^@/src(/.*|$)'],
            ['^@/app(/.*|$)'],
            ['^@/shared(/.*|$)'],
            ['^@/contexts(/.*|$)'],
            ['^'],
            ['^\\.'],
          ],
        },
      ],
    },
  },

  // JavaScript files - disable type checking
  {
    files: ['**/*.js', '**/*.mjs', '**/*.cjs'],
    languageOptions: {
      globals: {
        console: 'readonly',
        process: 'readonly',
        Buffer: 'readonly',
        __dirname: 'readonly',
        __filename: 'readonly',
        global: 'readonly',
        module: 'readonly',
        require: 'readonly',
        exports: 'readonly',
      },
    },
    rules: {
      '@typescript-eslint/no-var-requires': 'off',
      '@typescript-eslint/explicit-function-return-type': 'off',
    },
  },

  // Module files
  {
    files: ['**/*.module.ts'],
    rules: {
      '@typescript-eslint/no-extraneous-class': 'off',
    },
  },

  // Scripts
  {
    files: ['scripts/**/*'],
    rules: {
      'no-console': 'off',
    },
  },

  // Test files
  {
    files: ['tests/**/*', 'test/**/*', '**/*.test.ts', '**/*.spec.ts'],
    languageOptions: {
      parser: typescriptParser,
      parserOptions: {
        project: './tsconfig.json',
      },
      globals: {
        console: 'readonly',
        process: 'readonly',
        Buffer: 'readonly',
        __dirname: 'readonly',
        __filename: 'readonly',
        global: 'readonly',
        module: 'readonly',
        require: 'readonly',
        exports: 'readonly',
        describe: 'readonly',
        it: 'readonly',
        expect: 'readonly',
        beforeEach: 'readonly',
        afterEach: 'readonly',
        beforeAll: 'readonly',
        afterAll: 'readonly',
        test: 'readonly',
        vi: 'readonly',
      },
    },
    plugins: {
      '@typescript-eslint': typescript,
      vitest,
    },
    rules: {
      ...vitest.configs.recommended.rules,
      '@typescript-eslint/unbound-method': 'off',
      'vitest/expect-expect': 'off',
      'vitest/no-standalone-expect': 'off',
      'no-undef': 'off',
    },
  },

  // Performance test files
  {
    files: ['tests/performance/**/*'],
    rules: {
      'unicorn/numeric-separators-style': 'off',
      'unicorn/no-anonymous-default-export': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      'no-undef': 'off',
    },
  },
];

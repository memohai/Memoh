// @ts-check
import vueParser from 'vue-eslint-parser'
import tseslint from 'typescript-eslint'
import vue from 'eslint-plugin-vue'

export default [
  ...tseslint.configs.recommended,
  ...vue.configs['flat/recommended'],
  { ignores: ['**/node_modules/**', '**/dist/**', '**/out/**', '**/cache/**', '**/target/**', '**/.toolkit/**', 'packages/sdk/src/**'] },
  {
    files: ['packages/**/*.{js,jsx,ts,tsx}', 'apps/**/*.{js,jsx,ts,tsx}'],
    languageOptions: {
      parserOptions: {
        ecmaVersion: 2022,
        sourceType: 'module',
        projectService: true,
      },
    },
    rules: {
      quotes: ['error', 'single'],
      semi: ['error', 'never'],
      '@typescript-eslint/no-unused-vars': ['error', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
        destructuredArrayIgnorePattern: '^_',
      }],
    },
  },
  {
    files: ['packages/**/*.vue', 'apps/**/*.vue'],
    languageOptions: {
      parser: vueParser,
      parserOptions: {
        ecmaVersion: 2022,
        sourceType: 'module',
        parser: {
          js: 'espree',
          ts: tseslint.parser,
        },
      },
    },
    rules: {
      quotes: ['error', 'single'],
      semi: ['error', 'never'],
      'vue/multi-word-component-names': 'off',
      'vue/require-default-prop': 'off',
      'vue/no-required-prop-with-default':'error',
      '@typescript-eslint/no-unused-vars': ['error', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
        destructuredArrayIgnorePattern: '^_',
      }],
    },
  },
  {
    files: ['apps/web/src/**/*.{ts,tsx,vue}'],
    ignores: [
      'apps/web/src/store/chat/sync/**',
      'apps/web/src/store/chat/transcript.ts',
      'apps/web/src/store/chat/transcript.test.ts',
    ],
    rules: {
      'no-restricted-imports': ['error', {
        patterns: [{
          group: [
            '**/store/chat/transcript',
            './chat/transcript',
            './transcript',
            '../transcript',
          ],
          message: 'Transcript mutation is owned by store/chat/sync; consume a sync facade or a read-only view.',
        }],
      }],
    },
  },
]

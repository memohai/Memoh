// @ts-check
import vueParser from 'vue-eslint-parser'
import tseslint from 'typescript-eslint'
import vue from 'eslint-plugin-vue'

export default [
  ...tseslint.configs.recommended,
  ...vue.configs['flat/recommended'],
  { ignores: ['**/node_modules/**', '**/dist/**', '**/cache/**', '**/target/**', 'packages/sdk/src/**'] },
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
    files: [
      'apps/web/src/pages/chat/components/tool-call-edit.vue',
      'apps/web/src/pages/chat/components/tool-call-write.vue',
      'apps/web/src/pages/home/components/tool-call-edit.vue',
      'apps/web/src/pages/home/components/tool-call-write.vue',
    ],
    rules: {
      'vue/no-v-html': 'off',
    },
  },
]

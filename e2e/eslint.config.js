import js from '@eslint/js'
import playwright from 'eslint-plugin-playwright'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['node_modules', 'playwright-report', 'test-results', '.tmp'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.ts'],
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    ...playwright.configs['flat/recommended'],
    files: ['specs/**/*.ts', 'setup/**/*.ts'],
    rules: {
      ...playwright.configs['flat/recommended'].rules,
      // E2E specs branch on cluster state and skip optional suites on purpose
      'playwright/no-conditional-in-test': 'off',
      'playwright/no-conditional-expect': 'off',
      'playwright/no-skipped-test': 'off',
      'playwright/expect-expect': 'off',
    },
  }
)

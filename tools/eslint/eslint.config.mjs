import js from '@eslint/js';
import { defineConfig, globalIgnores } from 'eslint/config';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default defineConfig([
  globalIgnores(['plz-out/**', 'third_party/**', '**/node_modules/**']),
  {
    files: ['**/*.{js,jsx,mjs,cjs,ts,tsx}'],
    extends: [js.configs.recommended, tseslint.configs.recommended],
    languageOptions: {
      globals: { ...globals.node, ...globals.browser },
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
  },
  { files: ['test/jest/**'], languageOptions: { globals: globals.jest } },
  // In a CommonJS file, require is the syntax.
  { files: ['**/*.{js,cjs}'], rules: { '@typescript-eslint/no-require-imports': 'off' } },
]);

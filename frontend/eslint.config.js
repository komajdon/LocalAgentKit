import js from '@eslint/js'
import jsxA11y from 'eslint-plugin-jsx-a11y'
import reactHooks from 'eslint-plugin-react-hooks'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', 'wailsjs'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: { globals: globals.browser },
    plugins: { 'react-hooks': reactHooks, 'jsx-a11y': jsxA11y },
    rules: {
      ...jsxA11y.flatConfigs.recommended.rules,
      'react-hooks/rules-of-hooks': 'error',
      'react-hooks/exhaustive-deps': 'warn',
      // The generated Wails types are loose, so the bindings are cast.
      '@typescript-eslint/no-explicit-any': 'off',
      // Used only on inputs that appear because the user asked to edit.
      'jsx-a11y/no-autofocus': 'off',
      // TypeScript already reports these, with underscore conventions.
      '@typescript-eslint/no-unused-vars': 'off',
    },
  },
)

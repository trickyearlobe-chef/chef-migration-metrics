import js from "@eslint/js";
import tseslint from "typescript-eslint";
import globals from "globals";

export default tseslint.config(
  // Recommended base rules from ESLint core.
  js.configs.recommended,

  // TypeScript-aware rules from typescript-eslint.
  ...tseslint.configs.recommended,

  // Global settings applied to all files.
  {
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.es2020,
      },
      parserOptions: {
        ecmaFeatures: { jsx: true },
      },
    },
  },

  // Project-specific rule overrides.
  {
    rules: {
      // Allow prefixing unused vars with _ (common React pattern for
      // destructured props and catch bindings).
      "@typescript-eslint/no-unused-vars": [
        "warn",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
        },
      ],

      // Allow explicit `any` as a warning rather than an error — the
      // codebase is being progressively typed and hard errors would block
      // CI on existing code.
      "@typescript-eslint/no-explicit-any": "warn",
    },
  },

  // Ignore build output and dependencies.
  {
    ignores: ["dist/**", "node_modules/**"],
  },
);

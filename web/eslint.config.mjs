import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";

// TypeScript の型検査では検出できない Promise と React Hooks の規則だけを扱う。
// 新しい規則は既存コードの誤検出を確認してから追加する。
export default tseslint.config(
  {
    ignores: [
      // openapi.yaml から生成されるため直接編集しない。
      "src/api/schema.d.ts",
      "src/rules/generated.ts",
      "dist/**",
      "node_modules/**",
      "playwright-report/**",
      "test-results/**",
    ],
  },
  {
    files: ["**/*.ts", "**/*.tsx"],
    extends: [tseslint.configs.base],
    // 不要になった eslint-disable コメントをエラーにする。
    linterOptions: { reportUnusedDisableDirectives: "error" },
    languageOptions: {
      // application と e2e の両方の tsconfig を明示する。
      parserOptions: {
        project: ["./tsconfig.json", "./tsconfig.e2e.json"],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // 未処理の Promise と不正な Promise コールバックを検出する。
      "@typescript-eslint/no-floating-promises": "error",
      "@typescript-eslint/no-misused-promises": "error",
      "@typescript-eslint/await-thenable": "error",

      // オブジェクトの意図しない文字列化を検出する。
      "@typescript-eslint/no-base-to-string": "error",
      "@typescript-eslint/no-unnecessary-type-assertion": "error",

      // 先頭の `_` は「受け取るが使わない」という意思表示である。
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          argsIgnorePattern: "^_",
          varsIgnorePattern: "^_",
          caughtErrorsIgnorePattern: "^_",
          destructuredArrayIgnorePattern: "^_",
        },
      ],
    },
  },
  {
    // Playwright の use を React hook と誤認しないよう、React 規則は src だけに適用する。
    files: ["src/**/*.ts", "src/**/*.tsx"],
    plugins: { "react-hooks": reactHooks },
    rules: {
      // hook の呼び出し位置と依存配列を検証する。
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "error",
    },
  },
);

import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";

// ここに並べたのは、**型検査に原理的にできないこと**だけである。
//
// 長いあいだ、この repo には lint が無かった。「lint の役目は型検査が担う」と
// 決めてあり、それは大筋で正しい——名前の間違いも形の食い違いも tsc が捕まえる。
// だが tsc が見ないものが二種類ある: **待ち忘れた Promise** と、**React の規則**
// である。どちらも型としては正しく、走らせて初めて壊れる。
//
// **入れなかったものの方が多い。** 実測すると recommendedTypeChecked は 265 件を
// 出したが、その 174 件は unbound-method で、うち 173 件はテストの
// `expect(obj.method)` である。set-state-in-effect の 23 件は、注釈で理由が
// 書かれた意図的な形だった。**信号より雑音が多い lint は、いずれ丸ごと切られる。**
// 増やすなら、そのとき出るものを全部直せると分かってからにする。
export default tseslint.config(
  {
    ignores: [
      // 生成物。openapi.yaml が正本であり、ここを直すことはない。
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
    // **効かなくなった抑止を残さない。** 規則を止めるコメントは、それが
    // 止めている規則が実際に鳴っているあいだだけ意味を持つ。コードが直って
    // 鳴らなくなったあとも残っていれば、次に本当に鳴るべきときを黙らせる。
    //
    // この repo には、**走ってもいない規則を抑止するコメントが 9 箇所**あった。
    // 規則を入れた以上、その 9 箇所は決定になる——ここで数え続ける。
    linterOptions: { reportUnusedDisableDirectives: "error" },
    languageOptions: {
      // **両方の project を名指しする。** e2e は自分の tsconfig を持ち、その名前は
      // `tsconfig.json` ではないので、自動では見つけてもらえない——落ちるのでは
      // なく「型情報の無いまま素通りする」形で静かに外れる。
      parserOptions: {
        project: ["./tsconfig.json", "./tsconfig.e2e.json"],
        tsconfigRootDir: import.meta.dirname,
      },
    },
    rules: {
      // **待ち忘れた Promise は、型としては正しい。** 落ちても画面には何も出ず、
      // 失敗したことすら分からない——この画面はほぼ全ての操作が API 呼び出しで
      // ある以上、ここが一番効く。
      "@typescript-eslint/no-floating-promises": "error",
      "@typescript-eslint/no-misused-promises": "error",
      "@typescript-eslint/await-thenable": "error",

      // `[object Object]` を綴らない。tsc は String(x) を止めない。
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
    // **React の規則は React のコードにだけ効かせる。**
    //
    // e2e は Playwright であり、あそこの `use` は fixture を引き渡す関数であって
    // React 19 の `use` hook ではない。名前だけで見る規則はそれを区別できず、
    // 「hook でない関数の中で hook を呼んでいる」と言う——**正しくない指摘を
    // 抑止コメントで黙らせると、抑止コメントの方が信用を失う。**
    files: ["src/**/*.ts", "src/**/*.tsx"],
    plugins: { "react-hooks": reactHooks },
    rules: {
      // 依存の一覧に足りないものがあると、効果は古い値を握ったまま動き続ける。
      // この repo には既にこの規則を名指しで抑止するコメントが 9 箇所あった
      // ——**規則の側が走っていないまま。**
      "react-hooks/rules-of-hooks": "error",
      "react-hooks/exhaustive-deps": "error",
    },
  },
);

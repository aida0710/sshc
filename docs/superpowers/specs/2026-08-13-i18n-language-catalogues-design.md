# i18n言語カタログ分割設計

## 目的

約2,200行の `web/src/i18n/messages.ts` を言語ごとに分割し、英語を正本として、登録済みの各言語に不足・余分なキーがないことを型検査と明示的なコマンドの両方で確認できるようにする。

## ファイル構成

```text
web/src/i18n/
├── messages.ts
├── catalogue.ts
├── catalogue.test.ts
└── messages/
    ├── en.ts
    └── ja.ts
```

- `messages/en.ts` は英語カタログと、そこから導出した `MessageKey` を持つ。
- `messages/ja.ts` は `Record<MessageKey, string>` を満たす完全な日本語カタログを持つ。
- `messages.ts` は既存の公開窓口を維持し、`en`、`ja`、`messages`、`MessageKey` を再公開する。
- `catalogue.ts` は英語と候補言語のキー集合を比較し、不足キーと余分なキーをソート済み配列で返す。

## 検査

- `npm run typecheck --prefix web` は日本語の不足キーと余分なキーをコンパイルエラーにする。
- `npm run check:i18n --prefix web` は登録済みの全非英語カタログを英語と比較する。
- 差分があれば、検査結果に言語、不足キー、余分なキーをまとめて表示する。
- 通常の `npm test --prefix web` にも同じ検査を含める。

## 境界

- 既存の翻訳文、キー、locale判定、フォールバック、import先は変更しない。
- 新しい依存は追加しない。
- 今回はプレースホルダ名の一致検査や遅延ロードを追加しない。
- 新しい言語はカタログを作るだけでなく、`messages.ts` とlocale定義へ明示的に登録する。

# API コードの生成

API 仕様は [`openapi.yaml`](openapi.yaml) で管理しています。仕様のバージョンは、固定して使用している `oapi-codegen v2.7.0` が正式に対応する OpenAPI 3.0.3 です。

以前は OpenAPI 3.1.0 を指定していましたが、実際にはジェネレータが対応する基本的な機能だけを使用していました。`const` を単一要素の `enum` に変更し、生成される Go と TypeScript の型を維持したまま、仕様とジェネレータの対応範囲を一致させています。

## 生成と検証

```sh
go generate ./cmd/sshc
go generate ./internal/api
npm run generate:api --prefix web
go test ./internal/api -count=1
npm run typecheck --prefix web
```

`go generate ./internal/api` は Go のモデルに加えて、
`web/src/api/validators.generated.ts` も生成します。Web の共通APIクライアントは、
OpenAPIに宣言されたJSON request/responseをpath・method・statusごとのschemaで検証します。
個別画面のvalidatorを追加せず、制約は原則として `openapi.yaml` に追加してください。

複数フィールド間の大小関係や排他的な入力のようにOpenAPI 3.0標準だけでは表せない制約には、
`x-sshc-less-than-or-equal` と `x-sshc-exactly-one` を使用します。これらの拡張もruntime generatorが読み取るため、
検証ロジックをTypeScript側へ重複して記述する必要はありません。

CLIはOpenAPIとは別の契約ですが、`cmd/sshc/internal/clispec` を正本として、
parser dispatch、help、Bash/Zsh/Fish補完の共通語彙を
`cmd/sshc/cli_contract.gen.go` へ同時に生成します。引数間の意味的な制約だけは、
各command parserの手書きコードに残します。

OpenAPI 3.1 固有の機能を追加する場合は、先にジェネレータの対応状況を確認し、Go と TypeScript の生成結果を比較してから仕様のバージョンを変更してください。

## Go の命名規則

`oapi-codegen v2.7.0` は camelCase のプロパティ名について、先頭文字だけを大文字にした Go のフィールド名を生成します。

| OpenAPI | Go |
| --- | --- |
| `id` | `Id` |
| `keyId` | `KeyId` |
| `transactionId` | `TransactionId` |

生成コードに合わせる必要がある場合は、[`models.gen.go`](../internal/api/models.gen.go) を直接編集せず、呼び出し側または OpenAPI 仕様を変更してください。`required` に含まれないプロパティは、`omitempty` を持つポインタとして生成されます。たとえば、`Problem.Detail` は `*string`、`KeyItem.Certificate` は `*KeyCertificate` です。

## 鍵 vault API の設計

- タイムスタンプは RFC 3339 文字列として扱います。応答を組み立てる側で RFC 3339 に整形し、Go と TypeScript の生成型は `string` に統一します。
- `kind` や `algorithm` などの値は文字列として生成し、Go の API 境界で許容値を検証します。生成型だけを入力検証には使用しません。
- `Problem.detail` には、長さを制限し、ホームディレクトリを伏せたエラーメッセージだけを格納します。鍵素材、パスフレーズ、トークン、絶対パスを含めてはいけません。
- `KeyCertificate.validBefore` は符号付き整数と `neverExpires` の組み合わせで表します。OpenSSH の無期限値 `2^64-1` は符号付き整数に収まらないため、`neverExpires: true` と `validBefore: 0` を返します。
- `POST /api/v1/keys/hardware-command` はディスクを変更しません。検証済みの JSON 本文を受け取り、GET 以外のリクエストと同じ CSRF 保護を適用するために POST を使用します。
- `IssueActionRequest` は `kind` と `target` だけを受け取ります。確認トークンに結び付ける evidence は、秘密鍵のパス、フィンガープリント、内容のダイジェスト、またはごみ箱エントリから、サーバーが発行時と使用時に再計算します。クライアントから evidence を受け取ると、画面に表示していない状態へトークンを結び付けられるためです。
- アクション種別は `internal/session` の定数 `private_key.reveal` と `trash.purge` を使用します。確認が必要な操作の名前は session パッケージで一元管理します。

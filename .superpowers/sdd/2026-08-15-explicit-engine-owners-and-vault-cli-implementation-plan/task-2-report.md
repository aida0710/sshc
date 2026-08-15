# Task 2 report: versioned atomic handoff

## 変更概要

- handoff を schema/protocol/owner/PID/version を含む versioned document に変更した。
- `Write` は同一ディレクトリの 0600 一時ファイルに JSON を書き、file `Sync`、`Close`、`Rename`、directory `Sync` の順で公開する。directory は 0700 にそろえ、失敗時の一時ファイルは削除する。
- `Read` は旧 `{url,secret}` 形式を受け入れず、owner、loopback URL、secret、PID、version、schema/protocol を検証する。
- schema/protocol は `errors.Is` で識別できる `handoff.ErrSchemaVersion` / `handoff.ErrProtocolVersion` を返す。CLI 共通の `readHandoff` は不一致時に「running app and CLI must use the same version; restart the app」と案内する。
- `app.Dependencies` に `Owner` と `PID` を追加し、production の `cmd/sshc` が `os.Getpid()` と desktop/headless owner を渡す。PID の暗黙 fallback は無い。
- `Run` は Build が発行した秘密を保持し、同じ secret の文書だけを cleanup する。

## 判断

- cleanup の所有権には URL/PID でなく実行ごとの secret を使った。URL/PID は将来再利用され得るが、secret は別実行で共有されないためである。
- protocol/schema mismatch に旧形式 fallback を置かなかった。推測された owner/protocol で稼働中 app に接続しないためである。
- public `Build` の戻り値は維持し、private `build` だけが cleanup 用 document を返すようにした。公開 API を不要に広げず、別実行の secret を読み直して誤削除することを防ぐ。

## 変更ファイル

- `internal/handoff/handoff.go`, `internal/handoff/handoff_test.go`
- `internal/app/run.go`, `internal/app/run_test.go`
- `cmd/sshc/main.go`, `cmd/sshc/status.go`, `cmd/sshc/status_test.go`, `cmd/sshc/connect.go`, `cmd/sshc/connect_test.go`
- `internal/acceptance/harness_test.go`（有効な固定 PID/owner を test fixture に注入）

## TDD (RED → GREEN)

1. RED: `go test ./internal/handoff` は新しい Handoff fields/constants/errors が未実装で build failure（`unknown field SchemaVersion`、`undefined handoff.ErrSchemaVersion`）。
2. GREEN: atomic writer、validator、typed mismatch error の実装後に `ok sshc/internal/handoff`。
3. RED: app test 追加後に `go test ./internal/app` は旧 `handoff.Write(directory, url, secret)` と未追加 `Dependencies.Owner/PID` により build failure。
4. GREEN: versioned document の配線と secret cleanup 後に `ok sshc/internal/app`。
5. RED: CLI mismatch test 追加後に `go test ./cmd/sshc` は `undefined: readHandoff`。
6. GREEN: 共通 helper と全 read path の置換後に `ok sshc/cmd/sshc`。

## 検証結果

- `go test ./internal/handoff ./internal/app ./cmd/sshc` — PASS
- `go build ./...` — PASS
- `git diff --check` — PASS
- `go test ./...` — PASS（この module の package listing は `sshc/cmd/sshc` のみを出力）

## Self-review

- 旧文書、unknown owner、schema/protocol mismatch、非 loopback URL、空 secret、PID 0、空 version をテストした。
- JSON decode、0600 file、0700 directory、一時ファイルなし、secret 不一致で非削除、Run 中に置換された document の保持をテストした。
- `engineStatus`、`unlock`、`runOpen`、`waitForHandoff`、接続 client が共通 helper を使うことを確認した。

## 懸念

- `Remove` は Read と Remove の間を跨ぐ外部書き込みまで compare-and-delete 化するものではない。通常の production engine は start lock により同時 writer を排除し、要求された secret 一致の cleanup と後発 document 保持は満たす。lock 外から handoff を直接操作する新しい writer を導入する場合は、共有 lock または OS 固有の compare-and-remove を設計する必要がある。

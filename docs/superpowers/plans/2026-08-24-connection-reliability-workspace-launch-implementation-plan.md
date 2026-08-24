# Connection Reliability and Home Workspace Launch Implementation Plan

**Goal:** SSH 接続が切れたときの状態を機械的に観測できるようにし、再接続の衝突と無制御な集中再試行を防ぐ。保存済み Workspace は Home に表示し、利用者が明示的に選んだときだけ全 pane を開く。

**Architecture:** `internal/terminal.Session` を接続ライフサイクルの正本にする。状態、試行回数、次回試行時刻、公開用の問題コードを `terminal.View` と OpenAPI に載せ、Web は定期的な一覧更新で状態を表示する。再接続待機は既存 backoff に session ID 由来の安定した jitter を加え、同じ session の reopen は pump 一本だけが所有する。Workspace 起動要求は App が一度だけ保持し、Home から Terminal へ移動した後に既存 restore API と pane 接続経路へ渡す。

**Tech Stack:** Go / Echo / OpenAPI / React / TypeScript / Vitest / Playwright / OpenSSH integration

## Constraints

- `ServerAliveInterval` と `ServerAliveCountMax` は引き続き OpenSSH 設定を正本とし、sshc 固有の既定値を上書きしない。
- 再接続は切れた shell の継続ではなく新しい shell を開く。画面上でもその境界を維持する。
- 認証失敗、host key 変更、設定不正など、利用者の操作が必要な失敗を無条件に再試行しない。
- raw error、接続先の秘密、入力、scrollback を状態 API や disk へ保存しない。
- 同じ host の複数 pane は正当な利用なので、alias 単位の重複接続は禁止しない。
- Home は一覧の取得だけで SSH 接続を開始しない。Workspace の `開く` が唯一の開始操作である。
- `web/src` の変更後は `internal/ui/dist` を production build で更新する。

## Acceptance Criteria

- [x] session API が `connecting`、`connected`、`reconnecting`、`exited` を返し、再接続中は試行回数と次回試行時刻を返す。
- [x] 既存の切断再接続が一つの session ID のまま動き、同時に複数の reopen を起動しない。
- [x] 既定の再接続待機に ±20% の安定した jitter が入り、設定画面の選択回数と停止操作を尊重する。
- [x] 再接続上限到達時は公開用問題コードを返し、raw error を API に出さない。
- [x] session 一覧は接続状態を定期更新し、失効した取得結果が新しい状態を上書きしない。
- [x] Home に保存済み Workspace の名前、pane 数、更新時刻を表示する。
- [x] Home を開いただけでは restore も SSH open も呼ばれず、明示的な `開く` で一度だけ起動する。
- [x] Workspace 起動後は Terminal へ移動し、保存済み layout と focused pane を復元する。
- [x] 一部 pane の接続失敗は他 pane を止めず、失敗 pane に状態を残す。
- [x] Go unit/race、Web unit/typecheck/lint、generated contract、production build、関連 E2E が成功する。

## Phases

### Phase 1: 接続ライフサイクル

- `terminal.Session` に公開状態と再接続 metadata を追加する。
- 既存の再帰的 reopen を単一 loop に整理し、停止と設定変更を各試行前に評価する。
- session ID を seed に backoff jitter を計算する純粋関数と境界テストを追加する。
- OpenAPI、HTTP mapping、Web validator、ConsoleList/TerminalView 表示を更新する。

### Phase 2: Home Workspace 起動

- Workspace API client を Home から利用する。
- pane 数を layout から数える純粋関数を追加する。
- App に一度限りの Workspace 起動要求を置き、TerminalWorkspace が受け取って restore する。
- 二重 click、route 再描画、失敗 pane を検査する。

### Phase 3: Verification

- fake process で切断、再接続成功、連続失敗、停止競合を検査する。
- OpenSSH integration で実SSH認証とSFTP相互運用を確認する。
- Home と Terminal の browser journey を追加する。
- 全生成物と埋め込み Web assets を更新する。

## Risks

- session 一覧の polling が不要な負荷を生む可能性がある。Terminal が存在するときだけ低頻度で更新し、世代番号で古い応答を捨てる。
- API state と WebSocket state は異なる。前者は SSH process、後者は browser attachment として別表示を維持する。
- Workspace の同時起動で session 上限へ達しうる。成功 pane は維持し、失敗 pane だけを失敗状態にする。
- jitter により表示上の最大時間が変わる。設定文言は厳密な合計秒ではなく概算として示す。

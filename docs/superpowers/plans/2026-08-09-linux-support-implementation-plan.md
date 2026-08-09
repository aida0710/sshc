# Linux 対応と Keychain 廃止 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `internal/platform` をビルドタグで darwin / linux に分け、Linux で動く sshc を作る。あわせて macOS の Keychain 経路を廃止する。

**Architecture:** macOS 固有でないもの（`OutputRunner`、`ssh-add` を叩く `KeyAgent`）を `internal/platform/process` へ移して共有する。プラットフォームごとに違うのは Toolchain の探索パス、ブラウザのプログラム、ログイン時起動、端末起動の 4 つだけになる。`cmd/sshc` の組み立てを GOOS ごとのファイルへ分ける。

**Tech Stack:** Go 1.26、`os/exec`、systemd user unit、xdg-open。新しいモジュール依存は追加しない。

## Global Constraints

- 新しいモジュール依存を追加しない。
- 外部プログラムは絶対パスで起動する。PATH は参照しない。
- 子プロセスの環境は `platform.MinimalEnvironment` で置き換える。
- テストは本物のブラウザ・systemd・端末・agent・`~/.ssh` に触れない。記録用ランナーが argv を受け取り、プロセスは起動しない。
- コメントは日本語（だ・である調）。日本語と英数字の間に半角スペースを入れる。1 行は全角換算で 92 桁以内。
- 各タスクの最後に `gofmt -l`、`go build ./...`、`go vet ./...` が通ること。
- `GOOS=linux go build ./...` と `GOOS=linux go vet ./...` は、タスク 2〜4 では通ること。
  タスク 5 でビルドタグを付けた時点から**意図的に落ちる**（それがタグの効いた証拠で
  ある）。タスク 9 で `wiring_linux.go` を足して回復し、以降つねに通ること。
  タスク 5〜8 の実装者とレビュアーは、この期間 `GOOS=linux` が落ちることを欠陥として
  扱わない。`internal/platform/linux/` 単体（`GOOS=linux go vet ./internal/platform/linux/`）
  はタスク 6 以降つねに通ること。

---

## File Structure

| パス | 責任 |
|---|---|
| `internal/platform/keyagent.go` | 変更。`AgentAddRequest.StoreInKeychain` を削除 |
| `internal/platform/process/command.go` | 新規。`macos/command.go` の移動 |
| `internal/platform/process/command_test.go` | 新規。`macos/command_test.go` の移動 |
| `internal/platform/process/keyagent.go` | 新規。`macos/keyagent.go` から Keychain を抜いた移動 |
| `internal/platform/process/keyagent_test.go` | 新規。`macos/keyagent_test.go` の移動 |
| `internal/platform/macos/*.go` | `//go:build darwin` を付与。command/keyagent は削除 |
| `internal/platform/linux/toolchain.go` | 新規 |
| `internal/platform/linux/browser.go` | 新規 |
| `internal/platform/linux/loginitem.go` | 新規。systemd user unit |
| `internal/platform/linux/terminal.go` | 新規。custom のみ |
| `cmd/sshc/wiring_darwin.go` | 新規。darwin の部品組み立て |
| `cmd/sshc/wiring_linux.go` | 新規。linux の部品組み立て |
| `cmd/sshc/main.go` | 変更。組み立てを `platformParts` 呼び出しに置換 |
| `.github/workflows/ci.yml` | 変更。ubuntu ジョブを追加 |

---

### Task 1: Keychain を廃止する

**Files:**
- Modify: `internal/platform/keyagent.go:30`
- Modify: `internal/platform/macos/keyagent.go:72-90`
- Modify: `internal/platform/macos/keyagent_test.go:100-115`
- Modify: `internal/keys/service.go:585-665`
- Modify: `internal/keys/service_test.go:560-600`
- Modify: `internal/httpserver/keys.go:375-395`
- Modify: `internal/httpserver/keys_test.go:465`
- Modify: `internal/application/keymove.go:39-41,247`
- Modify: `internal/application/keymove_test.go:120-128`
- Modify: `internal/api/contract_test.go:130-142`
- Modify: `api/openapi.yaml:1895-1915`
- Modify: `web/src/keys/api.ts:65-75,201`
- Modify: `web/src/keys/KeysScreen.tsx:101,195,302,385,1055-1060`
- Modify: `web/src/keys/KeysScreen.test.tsx:151,177,486,529`
- Modify: `web/src/i18n/messages.ts:783,863,1627,1707`

**Interfaces:**
- Consumes: なし（最初のタスク）
- Produces: `platform.AgentAddRequest{PrivateKeyPath string; Passphrase []byte; LifetimeSeconds int}` — `StoreInKeychain` が無くなった形。タスク 3 がこれを移す。

- [ ] **Step 1: 杭になるテストを書く**

これは RED を作るためのテストではない。消す挙動が将来戻ってこないための杭であり、
書いた時点で通る。RED はこの直後の Step 3 で、`platform.AgentAddRequest` から
フィールドを消したときに、既存の `keyagent_test.go:107` が
`unknown field StoreInKeychain` でコンパイルできなくなる形で現れる。

`internal/platform/macos/keyagent_test.go` の `assertScrubbedEnvironment` を使うテストの隣に足す。

```go
// Keychain 経路は廃止した。パスフレーズの保存先は自前の vault ひとつであり、
// ssh-add に二つ目の保管場所を持たせない。--apple-use-keychain が復活すれば、
// 鍵を移動したときに絶対パスで識別された項目が壊れる問題も一緒に戻ってくる。
func TestAddNeverAsksSshAddToStoreThePassphrase(t *testing.T) {
	recorder := &recordingRunner{}
	agent := macos.NewKeyAgent(recorder, installedToolchain(), agentLookup)

	err := agent.Add(context.Background(), platform.AgentAddRequest{
		PrivateKeyPath:  "/home/u/.ssh/id_ed25519",
		Passphrase:      []byte("secret"),
		LifetimeSeconds: 3600,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range recorder.commands {
		for _, argument := range command.Arguments {
			if strings.Contains(argument, "keychain") {
				t.Fatalf("argv がキーチェーンに触れている: %#v", command.Arguments)
			}
		}
	}
}
```

- [ ] **Step 2: 杭が通ることを確認する**

Run: `go test ./internal/platform/macos/ -run TestAddNeverAsksSshAddToStoreThePassphrase -count=1`
Expected: PASS。`Add` はまだ `--apple-use-keychain` を足しうるが、このテストは
`StoreInKeychain` を立てていないので通る。ここで落ちるなら、`Add` が条件を見ずに
引数を足していることになり、それ自体が別の欠陥である。

- [ ] **Step 3: `platform.AgentAddRequest` からフィールドを消す**

`internal/platform/keyagent.go`:

```go
// AgentAddRequest はエージェントに秘密鍵を 1 つ読み込ませる。
//
// Passphrase は子プロセスの標準入力を通る。引数になることも環境変数になることも
// 決してない。どちらも、同じユーザーで動くどのプロセスからも読めるもの
// だからである。
type AgentAddRequest struct {
	PrivateKeyPath  string
	Passphrase      []byte
	LifetimeSeconds int
}
```

同ファイルの `KeyAgent` インターフェースのコメントから「そして macOS ではログインキーチェーンにも」を削る:

```go
// KeyAgent は、ユーザーの ssh-agent に秘密鍵を登録する。自動テストは常に偽物で
// 差し替える。このリポジトリのどのテストも本物のエージェントとは話さない。
```

- [ ] **Step 4: `macos/keyagent.go` から分岐を消す**

`Add` の中の以下 3 行を削除する:

```go
	if request.StoreInKeychain {
		arguments = append(arguments, "--apple-use-keychain")
	}
```

ファイル冒頭の doc コメントから `--apple-use-keychain` の段落（`// --apple-use-keychain は、パスフレーズを…` の 2 行と、その前後の空 `//` 行）を削除する。

- [ ] **Step 5: Go 側の呼び出し元を直す**

`internal/keys/service.go` の `RegisterRequest` から `StoreInKeychain bool` を、`RegisterResult` から `StoredInKeychain bool` を削除。`Register` の中の `StoreInKeychain: request.StoreInKeychain,` と `StoredInKeychain: request.StoreInKeychain,` の 2 行を削除。

`internal/httpserver/keys.go` の `StoreInKeychain: body.StoreInKeychain,` と `StoredInKeychain: result.StoredInKeychain,` を削除。

`internal/application/keymove.go` から `NoteKeychainEntryStale` の定数（39-41 行）と、247 行の `result.Notes = append(result.Notes, NoteKeychainEntryStale)` を含む条件ブロックを削除。

テスト側（`keyagent_test.go:107`、`keys/service_test.go:569,581,595`、`httpserver/keys_test.go:465`、`api/contract_test.go:134,139`、`application/keymove_test.go:120-128`）から該当の表明を削除する。`keymove_test.go` は Notes を数える表明なので、`len(result.Notes) != 0` に変える。

- [ ] **Step 6: openapi と生成物**

`api/openapi.yaml` の `RegisterKeyRequest` から `required` の `storeInKeychain` と `storeInKeychain: { type: boolean }` を、`RegisterKeyResult` から `required` の `storedInKeychain` と `storedInKeychain: { type: boolean }` を削除。

Run: `make generate`

- [ ] **Step 7: 画面から消す**

`web/src/keys/api.ts`: `storeInKeychain: boolean;` とその上のコメント 2 行、`asBoolean(record.storedInKeychain);` の行を削除。

`web/src/keys/KeysScreen.tsx`: `keychain_entry_stale:` の対応表の行、`const [storeInKeychain, setStoreInKeychain] = useState(false);`、`setStoreInKeychain(false);`、`storeInKeychain,` の送信フィールド、1055-1060 行のチェックボックスを削除。

`web/src/i18n/messages.ts`: `keys.storeInKeychain` と `keys.noteKeychainEntryStale` を en / ja の両方から削除。

`web/src/keys/KeysScreen.test.tsx`: 151・177 行の該当プロパティ、486・529 行の期待値、`registers a key with the agent, with the lifetime and Keychain choice the user made` のケースからチェックボックス操作を削除し、テスト名を `registers a key with the agent and the lifetime the user chose` に変える。627 行の `login Keychain` を探す表明を削除。

- [ ] **Step 8: 全体が通ることを確認する**

Run:
```bash
gofmt -l $(git ls-files '*.go' | grep -v models.gen.go)
go build ./... && go vet ./... && go test -count=1 ./...
make verify-generated
cd web && npm run typecheck && npm test
```
Expected: すべて成功。`grep -ri keychain --include='*.go' --include='*.ts' --include='*.tsx' --include='*.yaml' . | grep -v ^./docs` が、`internal/platform/macos/keyagent_test.go` の新しい杭テストだけを返すこと。

- [ ] **Step 9: コミット**

```bash
git add -A
git commit -m "Keychain 経路を廃止する

パスフレーズの保存先を自前の vault ひとつにする。ssh-add の
--apple-use-keychain は二つ目の保管場所であり、同じことを二か所で覚えていた。
しかも Keychain の項目は絶対パスで識別されるので鍵を移動すると壊れ、この
アプリケーションは Keychain を読み書きしないため警告することしかできなかった。

消えたのは argv の 1 引数だけではない。API の storeInKeychain と
storedInKeychain、keychain_entry_stale の注意、Keys 画面のチェックボックス、
そして keymove が返していた注意である。"
```

---

### Task 2: プロセス実行を共有パッケージへ移す

**Files:**
- Create: `internal/platform/process/command.go`（`internal/platform/macos/command.go` の移動）
- Create: `internal/platform/process/command_test.go`（`internal/platform/macos/command_test.go` の移動）
- Delete: `internal/platform/macos/command.go`, `internal/platform/macos/command_test.go`
- Modify: `cmd/sshc/main.go:239`

**Interfaces:**
- Consumes: なし
- Produces: `process.NewOutputRunner() platform.OutputRunner` — タスク 3・5・6 が使う。

- [ ] **Step 1: ファイルを移す**

```bash
mkdir -p internal/platform/process
git mv internal/platform/macos/command.go internal/platform/process/command.go
git mv internal/platform/macos/command_test.go internal/platform/process/command_test.go
```

- [ ] **Step 2: パッケージ名を変える**

`internal/platform/process/command.go` の 1 行目を `package macos` から `package process` に。
`internal/platform/process/command_test.go` の 1 行目を `package macos_test` から `package process_test` に変え、import の `"sshc/internal/platform/macos"` を `"sshc/internal/platform/process"` に、本文の `macos.` を `process.` に置換する。

`command.go` の doc コメントを実態に合わせる:

```go
// OutputRunner は、argv を直接指定して外部プログラムを実行する。
//
// シェルを起動することは決してなく、子プロセスが端末を読めないよう常に固定の
// 標準入力を与え、どちらのストリームについても platform.MaxCapturedOutput
// バイトを超えて保持することはない。
//
// このパッケージに macOS 固有のものは何もない。os/exec だけで書かれており、
// プラットフォームごとに違うのは、ここが起動するプログラムのパスの方である。
```

`NewOutputRunner` のコメントも `// NewOutputRunner はプロセスアダプタを返す。` に変える。

- [ ] **Step 3: 呼び出し元を直す**

`cmd/sshc/main.go` の `macos.NewOutputRunner()` を `process.NewOutputRunner()` に変え、import に `"sshc/internal/platform/process"` を足す。**呼び出しは 2 か所ある** — 239 行の `runner := …` と、144 行 `runOpen` の中の `macos.NewBrowser(macos.NewOutputRunner())`。後者は今回は `macos.NewBrowser(process.NewOutputRunner())` になる（Browser 自体はタスク 5 まで macos に残る）。`grep -n 'NewOutputRunner' cmd/sshc/*.go` で数を確かめること。

- [ ] **Step 4: 通ることを確認する**

Run: `gofmt -l $(git ls-files '*.go' | grep -v models.gen.go) && go build ./... && go vet ./... && go test -count=1 ./internal/platform/... ./cmd/sshc/`
Expected: すべて成功。

- [ ] **Step 5: コミット**

```bash
git add -A
git commit -m "プロセス実行を共有パッケージへ移す

command.go に macOS 固有のものは何もなかった。os/exec だけで書かれており、
プラットフォームごとに違うのは、ここが起動するプログラムのパスの方である。"
```

---

### Task 3: KeyAgent を共有パッケージへ移す

**Files:**
- Create: `internal/platform/process/keyagent.go`（`internal/platform/macos/keyagent.go` の移動）
- Create: `internal/platform/process/keyagent_test.go`（`internal/platform/macos/keyagent_test.go` の移動）
- Delete: `internal/platform/macos/keyagent.go`, `internal/platform/macos/keyagent_test.go`
- Modify: `cmd/sshc/main.go:267`

**Interfaces:**
- Consumes: `process.NewOutputRunner()`（タスク 2）、`platform.AgentAddRequest`（タスク 1 の形）
- Produces: `process.NewKeyAgent(runner platform.OutputRunner, toolchain platform.Toolchain, lookup func(string) (string, bool)) platform.KeyAgent` — タスク 5・6 が使う。

- [ ] **Step 1: ファイルを移す**

```bash
git mv internal/platform/macos/keyagent.go internal/platform/process/keyagent.go
git mv internal/platform/macos/keyagent_test.go internal/platform/process/keyagent_test.go
```

- [ ] **Step 2: パッケージ名を変える**

両ファイルの `package macos` / `package macos_test` を `process` / `process_test` に。テスト側の import と `macos.` 参照を `process.` に置換する。

`keyagent.go` の doc コメントの冒頭を、Keychain が消えた実態に合わせる:

```go
// KeyAgent は ssh-add を駆動する。
//
// ssh-add は、標準入力が利用できるならそこからパスフレーズを読む。したがって
// このアダプタには SSH_ASKPASS も端末も不要であり、秘密が argv や環境に届くことは
// 決してない。呼び出しのたびに子プロセスの環境を platform.MinimalEnvironment で
// 置き換えるのは、SSH_ASKPASS が SSH_ASKPASS_REQUIRE=force と組み合わさると、
// ssh-add がその標準入力を無視して、このアプリケーションが選んだのではない
// プログラムにパスフレーズを尋ねてしまうからだ。鍵のパスは常にワークスペース内の
// 絶対パスなので、オプションとして読まれることは決してない。
//
// プログラムのパスは定数ではなく Toolchain から来る。そのためこのアダプタは、
// アプリケーションの他の部分と同じ OpenSSH を実行し、PATH に依存することは
// 決してない。ssh-add はどのプラットフォームでも同じ引数を取るので、ここに
// プラットフォーム固有のものはない。
```

タスク 1 で足した杭テスト `TestAddNeverAsksSshAddToStoreThePassphrase` も一緒に移る。

- [ ] **Step 3: 呼び出し元を直す**

`cmd/sshc/main.go` の `KeyAgent: macos.NewKeyAgent(runner, toolchain, os.LookupEnv),` を `KeyAgent: process.NewKeyAgent(runner, toolchain, os.LookupEnv),` に。

- [ ] **Step 4: 通ることを確認する**

Run: `gofmt -l $(git ls-files '*.go' | grep -v models.gen.go) && go build ./... && go vet ./... && go test -count=1 ./...`
Expected: すべて成功。

- [ ] **Step 5: コミット**

```bash
git add -A
git commit -m "KeyAgent を共有パッケージへ移す

Keychain の分岐を落としたので、残るのは ssh-add を叩くだけの汎用コードである。
ssh-add はどのプラットフォームでも同じ引数を取る。"
```

---

### Task 4: Toolchain の探索パスを外から与える

**Files:**
- Modify: `internal/platform/macos/toolchain.go:27-29`
- Modify: `internal/platform/macos/toolchain_test.go`（存在すれば）

**Interfaces:**
- Consumes: なし
- Produces: `macos.NewToolchain() Toolchain` は据え置き。`Toolchain{Directories: []string{...}, Stat: nil}` を直接組み立てる形が linux 側で使える。

- [ ] **Step 1: 杭になるテストを書く**

これも RED を作るテストではない。書いた時点で通る。linux 版を足すときに macOS 側の
探索順が黙って変わらないよう留めるためのものである。

`internal/platform/macos/toolchain_test.go` に足す（無ければ作る。`package macos_test`）:

```go
// 探索順は固定であり、PATH は参照しない。このアプリケーションが実行する
// プログラムが、継承した環境に依存してはならない。
func TestToolchainSearchesFixedDirectoriesInOrder(t *testing.T) {
	want := []string{"/usr/bin", "/opt/homebrew/bin", "/usr/local/bin"}
	if got := macos.NewToolchain().Directories; !slices.Equal(got, want) {
		t.Fatalf("Directories = %#v, want %#v", got, want)
	}
}
```

- [ ] **Step 2: テストを走らせる**

Run: `go test ./internal/platform/macos/ -run TestToolchainSearchesFixedDirectoriesInOrder -count=1`
Expected: PASS。`slices` の import を足すこと。

- [ ] **Step 3: コミット**

```bash
git add internal/platform/macos/toolchain_test.go
git commit -m "Toolchain の探索順を杭で留める

linux 版を足すときに、macOS 側の順序が黙って変わらないようにする。"
```

---

### Task 5: macOS にビルドタグを付け、組み立てを分ける

**Files:**
- Modify: `internal/platform/macos/browser.go`, `toolchain.go`, `loginitem.go`, `terminal.go`（および各 `_test.go`）— 先頭に `//go:build darwin`
- Create: `cmd/sshc/wiring_darwin.go`
- Modify: `cmd/sshc/main.go:236-270`

**Interfaces:**
- Consumes: `process.NewOutputRunner()`、`process.NewKeyAgent(...)`
- Produces:

```go
// platformParts は、このプラットフォームの部品一式である。
type platformParts struct {
	Runner    platform.OutputRunner
	Toolchain platform.Toolchain
	Browser   platform.BrowserLauncher
	KeyAgent  platform.KeyAgent
	Terminal  platform.TerminalLauncher
	LoginItem httpserver.LoginItemController
}

func newPlatformParts(home string) platformParts
```

タスク 6 が linux 版の同じ関数を書く。

- [ ] **Step 1: ビルドタグを付ける**

`internal/platform/macos/` の残る全 `.go` ファイル（`browser.go`、`toolchain.go`、`loginitem.go`、`terminal.go` と対応する `_test.go`）の 1 行目に、空行を挟んで次を置く:

```go
//go:build darwin

package macos
```

- [ ] **Step 2: 落ちることを確認する**

Run: `GOOS=linux go build ./... 2>&1 | head`
Expected: FAIL。`cmd/sshc/main.go` が `macos` のシンボルを見つけられない。これがタグの効いた証拠である。

- [ ] **Step 3: `cmd/sshc/wiring.go`（タグなし）で型を定める**

型は GOOS ごとのファイルに置かない。同じパッケージなので二重定義になる。

```go
package main

import (
	"sshc/internal/httpserver"
	"sshc/internal/platform"
)

// platformParts は、このプラットフォームの部品一式である。
//
// 組み立てを GOOS ごとのファイルへ分けてあるのは、macOS のバイナリに Linux の
// コードが、Linux のバイナリに AppleScript の定数が入らないようにするためである。
// 実行時に runtime.GOOS で分岐すれば両方が入る。何が出荷物に入るかは、この
// アプリケーションが気にしてきたことである。
type platformParts struct {
	Runner    platform.OutputRunner
	Toolchain platform.Toolchain
	Browser   platform.BrowserLauncher
	KeyAgent  platform.KeyAgent
	Terminal  platform.TerminalLauncher
	LoginItem httpserver.LoginItemController
}
```

- [ ] **Step 4: `cmd/sshc/wiring_darwin.go` を作る**

```go
//go:build darwin

package main

import (
	"os"

	"sshc/internal/platform/macos"
	"sshc/internal/platform/process"
)

func newPlatformParts(home string) platformParts {
	// OpenSSH のプログラムを起動するすべてのサブシステムが、ひとつのプロセス
	// ランナーとひとつのツールチェーンを共有する。これにより argv、子プロセスの
	// 環境、出力の上限を決める場所はひとつだけになる。
	runner := process.NewOutputRunner()
	toolchain := macos.NewToolchain()
	return platformParts{
		Runner:    runner,
		Toolchain: toolchain,
		Browser:   macos.NewBrowser(runner),
		KeyAgent:  process.NewKeyAgent(runner, toolchain, os.LookupEnv),
		Terminal:  macos.NewTerminal(runner, home),
		LoginItem: macos.LoginItem{Runner: runner, Home: home},
	}
}
```

- [ ] **Step 5: `main.go` を書き換える**

236-270 行付近の組み立てを次に置き換える:

```go
	parts := newPlatformParts(home)

	var browser platform.BrowserLauncher = parts.Browser
	if !*openBrowser {
		browser = urlPrinter{out: os.Stdout}
	}

	dependencies := app.Dependencies{
		Random:  rand.Reader,
		Browser: browser,
		// ユーザーがインターフェースから有効にしない限りオフ。ここでは何も登録しない。
		// スイッチに手が届くようにするだけである。
		LoginItem: parts.LoginItem,
		// このアプリケーションが自分自身以外のホストに接触する唯一の場所であり、
		// 誰かが求めたときにだけ行う。何も取得せず、何も置き換えない。
		// 新しいバージョンが公開されているかを報告するだけである。
		Updates: &selfupdate.Checker{
			API:  "https://api.github.com/repos/aida0710/sshc/releases/latest",
			HTTP: &http.Client{Timeout: 30 * time.Second},
		},
		Listen:    net.Listen,
		UI:        assets,
		Logger:    logger,
		Home:      home,
		Runner:    parts.Runner,
		Toolchain: parts.Toolchain,
		KeyAgent:  parts.KeyAgent,
		Terminal:  parts.Terminal,
		Lookup:    os.LookupEnv,
```

以降（`AskpassHelper` など）は変更しない。

**`cmd/sshc` から `macos.` の参照を 1 つ残らず消すこと。** タグを付けた以上、1 つでも
残れば `GOOS=linux` では永久にビルドできない。タスク 2・3 の移動後に残っているのは
次の 4 か所である（行番号は移動前のもの。実際の位置は `grep -n 'macos\.' cmd/sshc/*.go`
で確かめること）:

- `main.go:144` `runOpen` の `macos.NewBrowser(macos.NewOutputRunner())`
  → `newPlatformParts(home).Browser`。この関数が `home` を持たないなら
  `os.UserHomeDir()` から取る。
- `main.go:186`、`main.go:201` の `macos.NewToolchain()` → `newPlatformParts(home).Toolchain`。
  ここも同様に `home` を用意する。
- `main.go:239-267` の組み立て（上記のとおり `parts` へ置換）。

`grep -n 'macos\.' cmd/sshc/*.go` が何も返さないこと、そして `main.go` の import から
`macos` が消えていることを確認する。

- [ ] **Step 6: darwin で通ることを確認する**

Run: `gofmt -l $(git ls-files '*.go' | grep -v models.gen.go) && go build ./... && go vet ./... && go test -count=1 ./...`
Expected: すべて成功。`GOOS=linux go build ./...` は依然 FAIL でよい（タスク 6 で解消する）。

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "macOS の実装にビルドタグを付け、組み立てを分ける

実行時の runtime.GOOS 分岐にはしない。macOS のバイナリに Linux のコードが、
Linux のバイナリに AppleScript の定数が入るのは、出荷物に何が入るかを気にする
このコードベースの姿勢と合わない。"
```

---

### Task 6: Linux の Toolchain とブラウザ

**Files:**
- Create: `internal/platform/linux/toolchain.go`
- Create: `internal/platform/linux/toolchain_test.go`
- Create: `internal/platform/linux/browser.go`
- Create: `internal/platform/linux/browser_test.go`

**Interfaces:**
- Consumes: `platform.OutputRunner`、`platform.Command`
- Produces: `linux.NewToolchain() linux.Toolchain` と `linux.NewBrowser(runner platform.OutputRunner) linux.Browser` — タスク 9 が使う。`Toolchain` は `platform.Toolchain` を、`Browser` は `platform.BrowserLauncher` を満たす。

- [ ] **Step 1: 失敗するテストを書く**

`internal/platform/linux/toolchain_test.go`:

```go
//go:build linux

package linux_test

import (
	"io/fs"
	"slices"
	"testing"

	"sshc/internal/platform/linux"
)

// 探索順は固定であり、PATH は参照しない。このアプリケーションが実行する
// プログラムが、継承した環境に依存してはならない。macOS 側と同じ理由である。
func TestToolchainSearchesFixedDirectoriesInOrder(t *testing.T) {
	want := []string{"/usr/bin", "/usr/local/bin", "/bin"}
	if got := linux.NewToolchain().Directories; !slices.Equal(got, want) {
		t.Fatalf("Directories = %#v, want %#v", got, want)
	}
}

// installedInfo は Stat の戻り値だけを満たす。中身は誰も見ない。
type installedInfo struct{ fs.FileInfo }

// 見つかった最初のディレクトリを返す。存在しないものは飛ばす。
func TestToolchainReturnsTheFirstDirectoryThatHasIt(t *testing.T) {
	toolchain := linux.Toolchain{
		Directories: []string{"/opt/absent", "/usr/bin"},
		Stat: func(path string) (fs.FileInfo, error) {
			if path == "/usr/bin/ssh" {
				return installedInfo{}, nil
			}
			return nil, fs.ErrNotExist
		},
	}
	got, err := toolchain.SSH()
	if err != nil || got != "/usr/bin/ssh" {
		t.Fatalf("SSH() = %q, %v", got, err)
	}
}
```

import は `"io/fs"`、`"slices"`、`"testing"`、`"sshc/internal/platform/linux"` の 4 つ。
`testing/fstest` は使わない。

`internal/platform/linux/browser_test.go`:

```go
//go:build linux

package linux_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

// browserRunner は、実行されたはずのコマンドを記録する。このパッケージのどのテストも
// 本物のブラウザを開かない。テストから開けば、デスクで動いている何かに生きた
// bootstrap トークンを渡すことになる。
type browserRunner struct{ commands []platform.Command }

func (runner *browserRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	runner.commands = append(runner.commands, command)
	return platform.Output{}, nil
}

func TestBrowserRunsXdgOpenByAbsolutePath(t *testing.T) {
	runner := &browserRunner{}
	target := "http://127.0.0.1:43123/#bootstrap=abc;$(touch%20/tmp/nope)"

	if err := linux.NewBrowser(runner).Open(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(runner.commands))
	}
	command := runner.commands[0]
	if command.Path != "/usr/bin/xdg-open" {
		t.Errorf("Path = %q, want the absolute path so PATH is never consulted", command.Path)
	}
	if !slices.Equal([]string{target}, command.Arguments) {
		t.Errorf("Arguments = %#v, want the URL as one element", command.Arguments)
	}
}

// ループバックの http 以外は開かない。URL は生きた bootstrap トークンを運ぶ。
func TestBrowserRefusesAnythingButLoopbackHTTP(t *testing.T) {
	for _, target := range []string{
		"https://example.com/", "http://example.com/",
		"http://192.168.1.10/", "file:///etc/passwd",
	} {
		runner := &browserRunner{}
		if err := linux.NewBrowser(runner).Open(context.Background(), target); !errors.Is(err, linux.ErrUnsafeBrowserURL) {
			t.Errorf("Open(%q) = %v, want ErrUnsafeBrowserURL", target, err)
		}
		if len(runner.commands) != 0 {
			t.Errorf("Open(%q) reached the process seam anyway", target)
		}
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `GOOS=linux go vet ./internal/platform/linux/`
Expected: FAIL。`linux` パッケージが存在しない。

- [ ] **Step 3: 実装する**

`internal/platform/linux/toolchain.go`:

```go
//go:build linux

package linux

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrProgramNotFound は、このアプリケーションがプログラムを実行してよいどの
// ディレクトリにも、その OpenSSH プログラムが入っていないことを報告する。
var ErrProgramNotFound = errors.New("openssh program not found")

// Toolchain は、固定の絶対パスで OpenSSH のプログラムを見つける。
//
// PATH は意図的に参照しない。このアプリケーションが実行するプログラムが、継承した
// 環境に依存してはならないからだ。macOS 側と同じ理由であり、違うのは並びだけである。
type Toolchain struct {
	Directories []string
	Stat        func(string) (fs.FileInfo, error)
}

// NewToolchain は、Linux の既定の探索順を返す。
func NewToolchain() Toolchain {
	return Toolchain{Directories: []string{"/usr/bin", "/usr/local/bin", "/bin"}}
}

// SSH は ssh クライアントの絶対パスを返す。
func (t Toolchain) SSH() (string, error) { return t.find("ssh") }

// KeyScan は ssh-keyscan の絶対パスを返す。
func (t Toolchain) KeyScan() (string, error) { return t.find("ssh-keyscan") }

// KeyGen は ssh-keygen の絶対パスを返す。
func (t Toolchain) KeyGen() (string, error) { return t.find("ssh-keygen") }

// KeyAdd は ssh-add の絶対パスを返す。
func (t Toolchain) KeyAdd() (string, error) { return t.find("ssh-add") }

func (t Toolchain) find(name string) (string, error) {
	stat := t.Stat
	if stat == nil {
		stat = os.Stat
	}
	for _, directory := range t.Directories {
		candidate := filepath.Join(directory, name)
		if _, err := stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s: %w", name, ErrProgramNotFound)
}
```

`internal/platform/macos/toolchain.go` の `find` と見比べ、シグネチャと戻り値の形を一致させること。

`internal/platform/linux/browser.go`:

```go
//go:build linux

package linux

import (
	"context"
	"errors"
	"net/url"

	"sshc/internal/platform"
)

var ErrUnsafeBrowserURL = errors.New("browser URL must use loopback HTTP")

// openProgram は、Linux が URL を既定のブラウザへ渡すためのプログラム。
//
// 絶対パスである。この URL は生きた bootstrap トークンを運ぶので、それを渡す相手が
// PATH の先頭にあるものであってはならない。他のすべての子プロセスと同じ規律である。
const openProgram = "/usr/bin/xdg-open"

// Browser は、既定のブラウザで URL を開く。
type Browser struct {
	runner platform.OutputRunner
}

func NewBrowser(runner platform.OutputRunner) Browser {
	return Browser{runner: runner}
}

// Open は、ループバックの http URL を既定のブラウザへ渡す。
//
// それ以外は拒否する。この URL はワンタイムの bootstrap トークンを運ぶので、
// 行き先はこのマシン上のこのプロセスだけでなければならない。シェルは介在せず、
// URL は 1 つの完全な引数として渡るので、その中の "$(...)" はただの文字である。
func (browser Browser) Open(ctx context.Context, target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		return ErrUnsafeBrowserURL
	}
	_, err = browser.runner.RunOutput(ctx, platform.Command{
		Path:      openProgram,
		Arguments: []string{target},
	})
	return err
}
```

- [ ] **Step 4: 通ることを確認する**

Run: `GOOS=linux go vet ./internal/platform/linux/ && GOOS=linux go test -count=1 ./internal/platform/linux/`

macOS 上では `GOOS=linux go test` は実行できない。Docker で確かめる:

```bash
docker run --rm -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/linux/ -count=1
```
Expected: PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/platform/linux/
git commit -m "Linux の Toolchain とブラウザ

探索順は固定で PATH は参照しない。ブラウザは /usr/bin/xdg-open を絶対パスで
呼び、ループバックの http 以外は拒否する。URL は生きた bootstrap トークンを運ぶ。"
```

---

### Task 7: Linux のログイン時起動（systemd user unit）

**Files:**
- Create: `internal/platform/linux/loginitem.go`
- Create: `internal/platform/linux/loginitem_test.go`

**Interfaces:**
- Consumes: `platform.OutputRunner`、`platform.Command`
- Produces: `linux.LoginItem{Runner platform.OutputRunner; Home string; Systemctl string}`。`internal/httpserver.LoginItemController` を満たす:

```go
Enabled() bool
Enable(ctx context.Context, program string) error
Disable(ctx context.Context) error
```

`Enabled()` は引数も error も取らない。読めない場合は false を返す。タスク 9 が使う。

- [ ] **Step 1: 失敗するテストを書く**

`internal/platform/linux/loginitem_test.go`:

```go
//go:build linux

package linux_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

// 本物の systemd を読み込むテストはない。ランナーは systemctl が何を求められた
// はずかを記録するだけで、何もしない。
type unitRunner struct{ commands []platform.Command }

func (runner *unitRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	runner.commands = append(runner.commands, command)
	return platform.Output{}, nil
}

// ログイン時には何も開かず、標準出力をどこにも送らない。エージェントが表示する
// URL は有効な bootstrap トークンを運び、journald はその置き場所ではない。
func TestEnableWritesAUnitThatOpensNothingAndLogsNothing(t *testing.T) {
	home := t.TempDir()
	runner := &unitRunner{}
	item := linux.LoginItem{Runner: runner, Home: home, Systemctl: "/usr/bin/systemctl"}

	if err := item.Enable(context.Background(), "/home/u/.local/bin/sshc"); err != nil {
		t.Fatal(err)
	}
	unit, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "sshc.service"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(unit)
	if !strings.Contains(text, "ExecStart=/home/u/.local/bin/sshc -open=false") {
		t.Errorf("unit does not start the agent with the browser off:\n%s", text)
	}
	if !strings.Contains(text, "StandardOutput=null") {
		t.Errorf("unit sends standard output somewhere:\n%s", text)
	}
}

// 絶対パスでなければ登録しない。systemd に PATH 経由で探させるプログラムは、
// 他人が供給しうるプログラムである。
func TestEnableRefusesARelativeProgram(t *testing.T) {
	runner := &unitRunner{}
	item := linux.LoginItem{Runner: runner, Home: t.TempDir(), Systemctl: "/usr/bin/systemctl"}

	if err := item.Enable(context.Background(), "sshc"); err == nil {
		t.Fatal("a relative program was registered")
	}
	if len(runner.commands) != 0 {
		t.Error("systemctl was run anyway")
	}
}

// 二度無効にすることは、呼び出し側が求めた状態である。
func TestDisableTwiceIsTheStateTheCallerAskedFor(t *testing.T) {
	home := t.TempDir()
	item := linux.LoginItem{Runner: &unitRunner{}, Home: home, Systemctl: "/usr/bin/systemctl"}

	if err := item.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := item.Disable(context.Background()); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `docker run --rm -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/linux/ -run TestEnable -count=1`
Expected: FAIL。`linux.LoginItem` が未定義。

- [ ] **Step 3: 実装する**

`internal/platform/macos/loginitem.go` を横に置いて、同じ構造で書く。plist の代わりに unit ファイル、`launchctl bootout/bootstrap` の代わりに `systemctl --user`。

```go
//go:build linux

package linux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshc/internal/platform"
)

// UnitName は、このアプリケーションが登録する systemd user unit の名前。
const UnitName = "sshc.service"

// ErrLoginItemPathNotAbsolute は、systemd が PATH 経由で探さなければならない
// プログラム、つまり他人が供給しうるプログラムの登録を拒否する。
var ErrLoginItemPathNotAbsolute = errors.New("login item program path must be absolute")

// LoginItem は「ログイン時に sshc を起動する」を切り替える。
//
// ユーザーが求めない限りオフである。保存済みのあらゆる秘密の鍵を握る
// バックグラウンドプロセスは、他人に代わって勝手に用意してよいものではないし、
// これがなくてもアプリケーションは十分に使える。何も動いていなければ
// `sshc <alias>` は素の ssh にフォールバックする。
//
// エージェントは -open=false で起動し、標準出力をどこにもリダイレクトしない。
// あの出力には有効な bootstrap トークン付きの URL が乗るので、journald はその
// 置き場所ではない。誰かが見たくなったときは `sshc open` が新しいものを発行する。
type LoginItem struct {
	// Runner は systemctl を実行する。テストが unit を読み込まないよう注入する。
	Runner platform.OutputRunner
	// Home はユーザーのホームディレクトリ。user unit が置かれる場所である。
	Home string
	// Systemctl はそのプログラム。argv を見たいテストのためにある。
	Systemctl string
}

func (l LoginItem) unitPath() string {
	return filepath.Join(l.Home, ".config", "systemd", "user", UnitName)
}

func (l LoginItem) systemctl() string {
	if l.Systemctl == "" {
		return "/usr/bin/systemctl"
	}
	return l.Systemctl
}

// Enabled は、unit が登録されているかを報告する。
//
// error を返さないのは、呼び出し側のインターフェースがそう決めているからである。
// 読めないことと登録されていないことは、この設定を表示する画面にとって同じ答えで
// ある。
func (l LoginItem) Enabled() bool {
	_, err := os.Stat(l.unitPath())
	return err == nil
}

// Enable は unit を書き出し、systemd に取り込ませる。
func (l LoginItem) Enable(ctx context.Context, program string) error {
	executablePath := program
	if !filepath.IsAbs(executablePath) || strings.ContainsAny(executablePath, "\n\r") {
		// 改行が入れば unit ファイルが壊れる。絶対パスでなければ systemd が
		// PATH 経由で探すことになり、それは他人が供給しうるプログラムである。
		return fmt.Errorf("%s: %w", executablePath, ErrLoginItemPathNotAbsolute)
	}
	if err := os.MkdirAll(filepath.Dir(l.unitPath()), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(l.unitPath(), []byte(unitFor(executablePath)), 0o600); err != nil {
		return err
	}
	// 先に daemon-reload するのは、以前のものを読み込んだままにせず、
	// プログラムパスの変更を拾わせるためである。
	if _, err := l.run(ctx, "--user", "daemon-reload"); err != nil {
		return err
	}
	_, err := l.run(ctx, "--user", "enable", "--now", UnitName)
	return err
}

// Disable は unit を止め、ファイルを取り除く。
func (l LoginItem) Disable(ctx context.Context) error {
	if _, err := l.run(ctx, "--user", "disable", "--now", UnitName); err != nil {
		return err
	}
	if err := os.Remove(l.unitPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := l.run(ctx, "--user", "daemon-reload")
	return err
}

func (l LoginItem) run(ctx context.Context, arguments ...string) (platform.Output, error) {
	return l.Runner.RunOutput(ctx, platform.Command{
		Path:      l.systemctl(),
		Arguments: arguments,
	})
}

// unitFor は systemd が読む unit ファイル。
//
// 標準出力を null へ送るのは、エージェントが表示する URL が有効な bootstrap
// トークンを運ぶからである。journald に残せば、それを読める者が入口を得る。
func unitFor(executablePath string) string {
	return "[Unit]\n" +
		"Description=sshc\n" +
		"\n" +
		"[Service]\n" +
		"Type=simple\n" +
		"ExecStart=" + executablePath + " -open=false\n" +
		"StandardOutput=null\n" +
		"StandardError=journal\n" +
		"Restart=on-failure\n" +
		"\n" +
		"[Install]\n" +
		"WantedBy=default.target\n"
}
```

`strings` を import すること。

- [ ] **Step 4: 通ることを確認する**

Run: `docker run --rm -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/linux/ -count=1`
Expected: PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/platform/linux/loginitem.go internal/platform/linux/loginitem_test.go
git commit -m "Linux のログイン時起動を systemd user unit で

macOS 版と同じく -open=false で起動し、標準出力をどこにも送らない。あの出力には
有効な bootstrap トークン付きの URL が乗るので、journald は置き場所ではない。"
```

---

### Task 8: Linux の端末起動（custom のみ）

**Files:**
- Create: `internal/platform/linux/terminal.go`
- Create: `internal/platform/linux/terminal_test.go`

**Interfaces:**
- Consumes: `platform.OutputRunner`、`platform.TerminalChoice`、`platform.ValidateTerminalChoice`
- Produces: `linux.NewTerminal(runner platform.OutputRunner) Terminal`。macos.Terminal と同じ面を満たす:

```go
Launch(ctx context.Context, alias string) error
LaunchIn(ctx context.Context, choice platform.TerminalChoice, alias string) error
LaunchWithPassword(ctx context.Context, alias, helperPath, endpoint, token string) error
LaunchWithPasswordIn(ctx context.Context, choice platform.TerminalChoice, alias, helperPath, endpoint, token string) error
Applications() []platform.Application
Terminals() []platform.TerminalAvailability
```

表を持たないので `Applications()` は空スライスを返し、`Terminals()` は
`TerminalCustom` の 1 件だけを返す。`Launch` と `LaunchWithPassword`（choice なし）は
`platform.ErrTerminalApplication` を返す — 何を開くかを利用者が指定していないので、
推測して開く先がない。タスク 9 が使う。

- [ ] **Step 1: 失敗するテストを書く**

```go
//go:build linux

package linux_test

import (
	"context"
	"slices"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

type terminalRunner struct{ commands []platform.Command }

func (runner *terminalRunner) RunOutput(_ context.Context, command platform.Command) (platform.Output, error) {
	runner.commands = append(runner.commands, command)
	return platform.Output{}, nil
}

// 端末の表は持たない。
//
// macOS では「CLI を持たない端末は Terminal.app と iTerm2 の二つで打ち止め」と
// 言い切れるので profile の表が意味を持つ。Linux では端末が乱立していて、実行する
// コマンドの渡し方も端末ごとに違う。表を用意すれば、そこに無い端末を使う人には
// 効かず、そこにある端末でも規約を取り違えれば黙って壊れる。推測しない。
func TestOnlyTheCustomTerminalIsOffered(t *testing.T) {
	if got := linux.NewTerminal(&terminalRunner{}).Applications(); len(got) != 0 {
		t.Fatalf("Applications() = %#v, want none", got)
	}
}

// 利用者が書いたコマンドが、そのまま argv として届く。シェルは介在しない。
func TestLaunchRunsTheChosenProgramWithTheAliasLast(t *testing.T) {
	runner := &terminalRunner{}
	terminal := linux.NewTerminal(runner)
	choice := platform.TerminalChoice{
		ID:          platform.TerminalCustom,
		Application: "/usr/bin/foot",
		Arguments:   []string{"-e"},
	}

	if err := terminal.LaunchIn(context.Background(), choice, "bastion"); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(runner.commands))
	}
	command := runner.commands[0]
	if command.Path != "/usr/bin/foot" {
		t.Errorf("Path = %q", command.Path)
	}
	if !slices.Contains(command.Arguments, "bastion") {
		t.Errorf("Arguments = %#v, want the alias", command.Arguments)
	}
}

// 安全な文字集合の外にある alias は起動しない。
func TestLaunchRefusesAnUnsafeAlias(t *testing.T) {
	runner := &terminalRunner{}
	choice := platform.TerminalChoice{
		ID: platform.TerminalCustom, Application: "/usr/bin/foot", Arguments: []string{"-e"},
	}
	if err := linux.NewTerminal(runner).LaunchIn(context.Background(), choice, "a;rm -rf /"); err == nil {
		t.Fatal("an unsafe alias was launched")
	}
	if len(runner.commands) != 0 {
		t.Error("the process seam was reached anyway")
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `docker run --rm -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/linux/ -run Terminal -count=1`
Expected: FAIL。`linux.NewTerminal` が未定義。

- [ ] **Step 3: 実装する**

`macos/terminal.go` の custom 分岐（378-404 行）を手本に、`open` を挟まず直接プログラムを起動する形で書く。要点:

- `platform.ValidateTerminalChoice(choice)` を必ず通す。
- `platform.ValidateAlias(alias)` を必ず通す。安全な文字集合の外は起動しない。
- `choice.Application` は絶対パスであること。そうでなければ `platform.ErrTerminalApplication`。
- 起動する argv は `[choice.Arguments..., <sshc の絶対パス>, alias]`。macOS 版が `program` を組み立てている箇所を読んで、同じ内容を渡すこと。
- `LaunchWithPassword` は `macos.TerminalPasswordScript` と同じ意味を持たせる。すなわち **環境変数やトークンをコマンド行に置かない**。`sshc <alias>` を実行させ、トークンはそのプロセスが handoff から取る。

- [ ] **Step 4: 通ることを確認する**

Run: `docker run --rm -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/linux/ -count=1`
Expected: PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/platform/linux/terminal.go internal/platform/linux/terminal_test.go
git commit -m "Linux の端末起動は custom のみ

端末の表は持たない。Linux では端末が乱立していて、実行するコマンドの渡し方も
端末ごとに違う。表を用意すれば、そこに無い端末を使う人には効かず、そこにある
端末でも規約を取り違えれば黙って壊れる。推測しない。"
```

---

### Task 9: Linux の組み立てと CI

**Files:**
- Create: `cmd/sshc/wiring_linux.go`
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`

**Interfaces:**
- Consumes: タスク 5 の `platformParts` 型と `newPlatformParts(home string) platformParts`、タスク 6・7・8 の `linux.*`
- Produces: なし（最終タスク）

- [ ] **Step 1: `cmd/sshc/wiring_linux.go` を書く**

`platformParts` の型はタスク 5 で `cmd/sshc/wiring.go`（タグなし）に置いてある。
ここに書くのは `newPlatformParts` だけである。

```go
//go:build linux

package main

import (
	"os"

	"sshc/internal/platform/linux"
	"sshc/internal/platform/process"
)

func newPlatformParts(home string) platformParts {
	// OpenSSH のプログラムを起動するすべてのサブシステムが、ひとつのプロセス
	// ランナーとひとつのツールチェーンを共有する。これにより argv、子プロセスの
	// 環境、出力の上限を決める場所はひとつだけになる。
	runner := process.NewOutputRunner()
	toolchain := linux.NewToolchain()
	return platformParts{
		Runner:    runner,
		Toolchain: toolchain,
		Browser:   linux.NewBrowser(runner),
		KeyAgent:  process.NewKeyAgent(runner, toolchain, os.LookupEnv),
		Terminal:  linux.NewTerminal(runner),
		LoginItem: linux.LoginItem{Runner: runner, Home: home},
	}
}
```

- [ ] **Step 2: Linux でビルドと vet が通ることを確認する**

Run: `GOOS=linux go build ./... && GOOS=linux go vet ./...`
Expected: どちらも成功。ここが Linux 対応の完了点である。

- [ ] **Step 3: Linux でテストが通ることを確認する**

Run: `docker run --rm -v "$PWD":/src -w /src golang:1.26 sh -c "go build ./... && go vet ./... && go test ./... -count=1"`
Expected: すべて PASS。落ちるものがあれば、それは Linux で成り立たない前提を持つテストなので、そのテスト自身を直す（新しいタスクを立てる）。

- [ ] **Step 4: CI に ubuntu ジョブを足す**

`.github/workflows/ci.yml` の `Go` ジョブの下に、同じ形で追加する:

```yaml
  linux:
    name: Linux
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
        with:
          go-version-file: go.mod

      # macOS のジョブと同じゲートを Linux でも回す。プラットフォーム層は
      # ビルドタグで分かれているので、片方だけを見ていると、もう片方が
      # コンパイルすら通らなくなったことに誰も気づかない。
      - name: Format
        run: test -z "$(gofmt -l $(git ls-files '*.go' | grep -v models.gen.go))"
      - name: Vet
        run: go vet ./...
      - name: Test
        run: go test ./... -count=1
```

既存のジョブが使っている actions の SHA をそのままコピーすること。

- [ ] **Step 5: README を直す**

「前提環境と固定バージョン」の `macOS arm64` を、macOS と Linux の両方が動くと読める形に改める。「セキュリティ境界」の節に、Keychain への言及が残っていれば削る（「Keychain だけは読み書きせず、`ssh-add` へ委譲するだけです」の一文）。「鍵管理の境界」の Keychain の項目（`ssh-add --apple-use-keychain` と、鍵の移動で項目が壊れる話）を削る。

Run: `grep -n -i keychain README.md`
Expected: 何も返らない。

- [ ] **Step 6: 全体が通ることを確認する**

Run:
```bash
gofmt -l $(git ls-files '*.go' | grep -v models.gen.go)
go build ./... && go vet ./... && go test -count=1 ./...
make verify-generated
cd web && npm run typecheck && npm test
```
Expected: すべて成功。

- [ ] **Step 7: コミット**

```bash
git add -A
git commit -m "Linux の組み立てと CI

GOOS=linux で build・vet・test が通る。CI に ubuntu のジョブを足したのは、
プラットフォーム層がビルドタグで分かれている以上、片方だけを見ていると
もう片方がコンパイルすら通らなくなったことに誰も気づかないからである。"
```

---

## Self-Review

**仕様の網羅**

| 仕様の節 | タスク |
|---|---|
| Keychain をやめる | 1 |
| 分け方（`process` へ移す） | 2, 3 |
| 分け方（ビルドタグ） | 5, 9 |
| Linux: Toolchain | 6 |
| Linux: Browser | 6 |
| Linux: LoginItem | 7 |
| Linux: Terminal | 8 |
| 権限とパス（変更なし） | — 変更が不要であることをタスク 9 Step 3 が確かめる |
| Windows をやらない | — 範囲外 |
| テスト（CI に ubuntu） | 9 |
| `make build` を変えない | — 変更しない |

**自己レビューで直したもの**

- タスク 1 と 4 の Step 1 を「失敗するテスト」と書いていたが、どちらも書いた時点で
  通る杭である。RED がどこで現れるかを本文に書き、見出しを直した。
- タスク 7 の `LoginItem.Enabled` を `(bool, error)` で書いていたが、実物の
  `httpserver.LoginItemController` は `Enabled() bool` である。実物に合わせた。
- タスク 8 の `LaunchIn` を仮の名前として逃げていたが、`SelectableTerminalLauncher`
  に実在する。満たすべき 6 メソッドを列挙した。
- タスク 6 のテストが `fstest.MapFile{}.Sys()` という動かないコードだった。
  `installedInfo` のスタブに置き換えた。
- `platformParts` の型をタスク 5 で作ってタスク 9 で移す形にしていた。最初から
  タグの付かない `cmd/sshc/wiring.go` に置く。

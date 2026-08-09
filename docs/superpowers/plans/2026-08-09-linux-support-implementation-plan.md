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

### Task 4b: Toolchain の機構を共有パッケージへ移す

仕様はこう書いている — 「`Toolchain` は機構と一覧を分ける。ディレクトリの列を
持って `Stat` で探すという中身は共有し、その列だけがプラットフォームごとに違う。」
当初のタスク 6 はこれに反して 57 行を `linux/toolchain.go` へ丸ごと複製していた。
しかもその複製の `find` は、本物にある `IsRegular()` と実行ビットの検査を落として
おり、ディレクトリや実行不能なファイルを `ssh` として受け入れてしまう。複製は
仕様違反であるだけでなく、劣化した複製だった。機構を先に共有側へ移す。

**Files:**
- Create: `internal/platform/process/toolchain.go`（`macos/toolchain.go` の移動）
- Create: `internal/platform/process/toolchain_test.go`（機構のテストの移動）
- Modify: `internal/platform/macos/toolchain.go` — 一覧だけを残す
- Modify: `internal/platform/macos/toolchain_test.go` — 一覧の杭だけを残す
- Modify: `internal/effective/evaluate_test.go`, `internal/effective/differential_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `process.Toolchain{Directories []string; Stat func(string) (fs.FileInfo, error)}` と `process.ErrProgramNotFound`。`platform.Toolchain` を満たす。`macos.NewToolchain() process.Toolchain` は据え置き。タスク 6 は 3 行の `linux.NewToolchain()` を書くだけになる。

- [ ] **Step 1: 機構を移す**

```bash
git mv internal/platform/macos/toolchain.go internal/platform/process/toolchain.go
```

`package macos` を `package process` に変える。`find` の中身は 1 文字も変えない
— `IsRegular()` と `Perm()&0o111` の検査はこの型が持つ意味そのものである。

型コメントから macOS 固有の 2 文（`/usr/bin` が最初な理由と Homebrew の話）を
外し、次に置き換える。あれは一覧の話であって機構の話ではない:

```go
// Toolchain は、固定の絶対パスで OpenSSH のプログラムを見つける。
//
// PATH は意図的に参照しない。このアプリケーションが実行するプログラムが、継承した
// 環境に依存してはならないからだ。どのディレクトリをどの順で見るかはプラット
// フォームごとに違うが、探し方は同じである。だからここには一覧を持たない。
type Toolchain struct {
	Directories []string
	Stat        func(string) (fs.FileInfo, error)
}
```

`NewToolchain` はこのファイルから消す。一覧はプラットフォーム側が持つ。

- [ ] **Step 2: macOS 側に一覧だけを残す**

`internal/platform/macos/toolchain.go` を次の内容にする:

```go
package macos

import "sshc/internal/platform/process"

// NewToolchain は、macOS の探索順を返す。
//
// /usr/bin が最初なのは、macOS に同梱される OpenSSH を設計上の対象にしている
// からである。Homebrew の prefix は、Apple のコピーが取り除かれたマシンのための
// フォールバックだ。探し方そのものは process.Toolchain が持つ。
func NewToolchain() process.Toolchain {
	return process.Toolchain{
		Directories: []string{"/usr/bin", "/opt/homebrew/bin", "/usr/local/bin"},
	}
}
```

- [ ] **Step 3: テストを分ける**

`internal/platform/macos/toolchain_test.go` から、機構を試す 3 つ
（`TestToolchainPrefersTheFirstDirectoryThatHoldsAnExecutable`、
`TestToolchainIgnoresMissingAndNonExecutableFiles`、
`TestToolchainResolvesEveryProgramThroughTheInjectedStat`）と
`var _ platform.Toolchain = macos.Toolchain{}` を
`internal/platform/process/toolchain_test.go`（`package process_test`）へ移す。
`macos.` を `process.` に置換し、表明は 1 つも変えない。宣言は
`var _ platform.Toolchain = process.Toolchain{}` になる。

`macos/toolchain_test.go` に残すのは一覧の杭 2 つ
（`TestNewToolchainLooksAtTheSystemOpenSSHFirst` と、タスク 4 で足した
`TestToolchainSearchesFixedDirectoriesInOrder`）だけである。不要になった import
を落とすこと。

- [ ] **Step 4: `internal/effective` のテストを直す**

`evaluate_test.go:178` と `differential_test.go:27` の `macos.NewToolchain()` は、
「このマシンに入っている本物の ssh を見つける、無ければスキップ」以上のことを
求めていない。macOS を名指しする理由がないので、共有の型で書く。
`internal/effective/differential_test.go` に helper を 1 つ置き、両方から使う:

```go
// systemToolchain は、このマシンに入っている本物の OpenSSH を探す。どちらの
// プラットフォームの既定の置き場所も含める。ここが求めているのは「本物の ssh が
// あるか、無ければスキップ」だけであり、プラットフォームの区別ではない。
func systemToolchain() process.Toolchain {
	return process.Toolchain{
		Directories: []string{"/usr/bin", "/opt/homebrew/bin", "/usr/local/bin", "/bin"},
	}
}
```

両ファイルの `macos.NewToolchain()` を `systemToolchain()` に変え、import の
`macos` を `process` に差し替える。

- [ ] **Step 5: 通ることを確認する**

Run:
```bash
gofmt -l $(git ls-files '*.go' | grep -v models.gen.go)
go build ./... && go vet ./... && go test -count=1 ./...
GOOS=linux go build ./... && GOOS=linux go vet ./...
```
Expected: すべて成功。`grep -rn 'macos' internal/platform/process/ internal/effective/` が
何も返さないこと。

- [ ] **Step 6: コミット**

```bash
git add -A
git commit -m "Toolchain の機構を共有パッケージへ移す

仕様は機構と一覧を分けよと言っている。ディレクトリの列を Stat で探すという
中身はどのプラットフォームでも同じで、違うのはその列だけである。

複製しなかったのは、複製が仕様違反だからだけではない。複製の find は本物に
ある IsRegular と実行ビットの検査を落としがちで、そうなればディレクトリを
ssh として受け入れる。検査は 1 か所にしかない方がよい。"
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
- Consumes: `process.Toolchain`（タスク 4b）、`platform.OutputRunner`、`platform.Command`
- Produces: `linux.NewToolchain() process.Toolchain` と `linux.NewBrowser(runner platform.OutputRunner) linux.Browser` — タスク 9 が使う。`Browser` は `platform.BrowserLauncher` を満たす。

- [ ] **Step 1: 失敗するテストを書く**

`internal/platform/linux/toolchain_test.go`:

```go
//go:build linux

package linux_test

import (
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
```

import は `"slices"`、`"testing"`、`"sshc/internal/platform/linux"` の 3 つだけ。

探し方そのもの（`Stat` で回して最初に見つかった実行可能ファイルを返す）を試す
テストはここには要らない。タスク 4b で `internal/platform/process` に移してあり、
そこで試されている。ここで試すのは、この一覧とこの順序だけである。

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

import "sshc/internal/platform/process"

// NewToolchain は、Linux の探索順を返す。
//
// PATH は意図的に参照しない。このアプリケーションが実行するプログラムが、継承した
// 環境に依存してはならないからだ。macOS 側と同じ理由であり、違うのは並びだけで
// ある。探し方そのものは process.Toolchain が持つ。
func NewToolchain() process.Toolchain {
	return process.Toolchain{
		Directories: []string{"/usr/bin", "/usr/local/bin", "/bin"},
	}
}
```

これで全部である。`Toolchain` 型も `find` も `ErrProgramNotFound` も書かない。
タスク 4b で `internal/platform/process` に移してあり、そこが唯一の実装である。

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

### Task 7b: 端末パスの形の検査をプラットフォームごとに分ける

`platform.ValidateTerminalChoice` は custom について
`filepath.Ext(choice.Application) != ".app"` を要求している。これは macOS の
アプリケーションバンドルの約束であって、端末が `/usr/bin/foot` のような ELF
実行ファイルである Linux には存在しない。

これはテストの都合ではない。この関数は保存経路から呼ばれており
（`internal/application/metadata.go:161,205`、`internal/application/service.go:232`）、
このままでは Linux の利用者は端末の設定を保存できない。仕様が「それ以外に、
Linux で欠ける機能はない」「端末は利用者がコマンドを書く」と言っている以上、
ここを分けないと Linux の端末機能は成立しない。

macOS の振る舞いは 1 ビットも変えない。`.app` の要求は darwin 側に残る。

**Files:**
- Modify: `internal/platform/terminal.go:86`
- Create: `internal/platform/application_darwin.go`
- Create: `internal/platform/application_linux.go`
- Create: `internal/platform/application_darwin_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `platform.ValidateTerminalChoice` の意味が GOOS で変わる。タスク 8 が
  `/usr/bin/foot` を通せるようになる。

- [ ] **Step 1: 失敗するテストを書く**

`internal/platform/application_darwin_test.go`（`package platform_test`、
`//go:build darwin`）に、darwin での既存の振る舞いが変わっていないことを留める:

```go
// バンドルの要求は macOS のものである。ここが緩めば、保存の時点で弾けたはずの
// 設定が起動の時点まで生き延びる。
func TestCustomTerminalOnDarwinMustBeAnApplicationBundle(t *testing.T) {
	for _, application := range []string{"/usr/bin/foot", "/Applications/Foo", "/Applications/Foo.APP"} {
		choice := platform.TerminalChoice{ID: platform.TerminalCustom, Application: application}
		if err := platform.ValidateTerminalChoice(choice); !errors.Is(err, platform.ErrTerminalApplication) {
			t.Errorf("ValidateTerminalChoice(%q) = %v, want ErrTerminalApplication", application, err)
		}
	}
	ok := platform.TerminalChoice{ID: platform.TerminalCustom, Application: "/Applications/Foo.app"}
	if err := platform.ValidateTerminalChoice(ok); err != nil {
		t.Errorf("ValidateTerminalChoice(bundle) = %v, want nil", err)
	}
}
```

これは書いた時点で通る杭である。RED は Step 3 で、フックを入れ違えたときに現れる。

- [ ] **Step 2: フックを切り出す**

`internal/platform/terminal.go` の custom の枝から拡張子の検査だけを外し、
フックを呼ぶ形にする:

```go
	if !filepath.IsAbs(choice.Application) ||
		filepath.Clean(choice.Application) != choice.Application ||
		!validApplicationPath(choice.Application) {
		return ErrTerminalApplication
	}
```

絶対パスと `Clean` の一致はどちらのプラットフォームでも要る検査なので、共有側に
残す。分けるのは形の約束だけである。

- [ ] **Step 3: プラットフォームごとの実体を書く**

`internal/platform/application_darwin.go`:

```go
//go:build darwin

package platform

import "path/filepath"

// validApplicationPath は、開く先が macOS のアプリケーションバンドルかを答える。
//
// macOS で端末を開くのは Launch Services であり、それが受け取るのはバンドルで
// ある。保存の時点でこれを要求するのは、起動の時点まで持ち越せば、設定した人が
// 間違いに気づくのが「開こうとしたとき」になるからだ。
func validApplicationPath(path string) bool {
	return filepath.Ext(path) == ".app"
}
```

`internal/platform/application_linux.go`:

パッケージは `platform` である（`linux` ではない）。

```go
//go:build linux

package platform

// validApplicationPath は、開く先の形について Linux で言えることを答える。
//
// Linux の端末は実行ファイルそのものであり、バンドルという約束はない。だから
// 拡張子について言えることは何もなく、形の検査は共有側の「絶対パスであること」
// と「Clean と一致すること」で尽きている。そこにあるか、実行できるかは起動する
// 側が見る。ここでディスクを見に行かないのは、アンインストールしただけで設定が
// 保存できなくなるのを避けるためで、これは macOS 側と同じ判断である。
func validApplicationPath(string) bool {
	return true
}
```

- [ ] **Step 4: 通ることを確認する**

Run:
```bash
gofmt -l $(git ls-files '*.go' | grep -v models.gen.go)
go build ./... && go vet ./... && go test -count=1 ./...
GOOS=linux go vet ./internal/platform/ ./internal/application/
```
Expected: すべて成功。

- [ ] **Step 5: コミット**

```bash
git add -A
git commit -m "端末パスの形の検査をプラットフォームごとに分ける

.app という拡張子の要求は macOS のアプリケーションバンドルの約束であって、
端末が実行ファイルそのものである Linux には存在しない。

これは検証のためだけの分岐ではない。この関数は設定の保存経路から呼ばれて
いるので、分けなければ Linux の利用者は端末をひとつも保存できない。

macOS の振る舞いは変えていない。絶対パスであることと Clean と一致することは
どちらのプラットフォームでも要るので共有側に残し、分けたのは形の約束だけで
ある。"
```

---

### Task 8: Linux では端末を開かない

方針を変えた。当初は `TerminalCustom` だけを実装するつもりだったが、Linux では
成立しないことが実装して分かった。

`RunOutput` は子プロセスの終了を待つ。macOS は `/usr/bin/open` を挟むので即座に
戻るが、Linux は端末エミュレータを直接起動する。`foot -e` も `xterm -e` も
ウィンドウが閉じるまで前面に留まるので、HTTP リクエストがセッションのあいだ
ずっと開いたままになる。しかも `exec.CommandContext` に渡っているのは
`c.Request().Context()` なので、利用者がタブを閉じた時点で SIGKILL が飛び、
使用中の SSH セッションごと落ちる。

待たない起動口を新設するか `setsid` を挟むかで直せるが、どちらも取らない。
Linux はこの機能を提供せず、利用者が自分でコマンドを実行する。

これは既に一級の設定である。`app.Dependencies.Terminal` は nil を許すと明記され
（`internal/app/run.go:50-52`）、`internal/diagnostics/service.go:238,331` が
panic せずに「端末が設定されていない」と報告する。

ただし nil にするだけでは足りない。2 か所が嘘をつく。

**Files:**
- Delete: `internal/platform/linux/terminal.go`, `internal/platform/linux/terminal_test.go`
- Modify: `internal/diagnostics/service.go` — `TerminalCommand` と `TerminalOptions`
- Modify: `internal/diagnostics/service_test.go`

**Interfaces:**
- Consumes: なし
- Produces: `linux` パッケージは端末を持たない。タスク 9 の `wiring_linux.go` は
  `Terminal` を設定しない（nil のまま）。

- [ ] **Step 1: 失敗するテストを書く**

`internal/diagnostics/service_test.go` に 2 つ足す。どちらも `Terminal` を nil に
した `Service` に対して行う。

```go
// 端末を持たないプラットフォームでは、開くボタンを出す根拠がない。
// コマンド自体は返す。利用者が自分で実行できるからであり、それが
// このプラットフォームでの答えである。
func TestTerminalCommandIsNotLaunchableWithoutALauncher(t *testing.T) {
	service := &diagnostics.Service{}
	command, launchable, warning := service.TerminalCommand("bastion")
	if command == "" {
		t.Error("コマンドは返すこと。利用者が自分で実行する")
	}
	if launchable {
		t.Error("launchable = true、端末を開く手段が無いのに")
	}
	if warning == "" {
		t.Error("なぜ開けないかを言うこと")
	}
}

// 在庫を「分からない」と扱ってはいけない。ランチャーが無いことは、
// 選択肢が無いと分かっていることである。
func TestTerminalOptionsOffersNothingWithoutALauncher(t *testing.T) {
	service := &diagnostics.Service{}
	available, applications, _ := service.TerminalOptions()
	if len(available) != 0 || len(applications) != 0 {
		t.Fatalf("TerminalOptions = %#v, %#v, want empty", available, applications)
	}
}
```

- [ ] **Step 2: 落ちることを確認する**

Run: `go test ./internal/diagnostics/ -run 'TestTerminal(CommandIsNotLaunchable|OptionsOffersNothing)WithoutALauncher' -count=1 -v`
Expected: FAIL。`TerminalCommand` は alias が安全なら無条件に `true` を返し、
`TerminalOptions` はランチャーが在庫を答えられないとき全部 installed と答える。

- [ ] **Step 3: 直す**

`TerminalCommand`（`internal/diagnostics/service.go:288`）に、alias の検証と同じ
高さで分岐を足す:

```go
	if s.Terminal == nil {
		return command, false, TerminalUnavailableWarning
	}
```

`UnsafeAliasWarning` の隣に定数を置く。文面は英語で、既存の警告と同じ調子にする
（例: `"This platform does not open a terminal for you. Run the command above yourself."`）。
`UnsafeAliasWarning` の綴りと置き場所を見て合わせること。

`TerminalOptions`（同 `:263`）の型アサーションの前に足す:

```go
	if s.Terminal == nil {
		return nil, nil, selected
	}
```

既存のコメントが「画面が選択肢を隠す根拠は『無いと分かっている』ことだけで、
『分からない』ことではない」と言っている。nil は前者である。そのことをコメントに
書き足すこと。

- [ ] **Step 4: Linux の端末を消す**

```bash
git rm internal/platform/linux/terminal.go internal/platform/linux/terminal_test.go
```

- [ ] **Step 5: 通ることを確認する**

Run:
```bash
gofmt -l $(git ls-files '*.go' | grep -v models.gen.go)
go build ./... && go vet ./... && go test -count=1 ./...
GOOS=linux go vet ./internal/platform/linux/ ./internal/diagnostics/
docker run --rm -v "$PWD":/src -w /src golang:1.26 go test ./internal/platform/linux/ ./internal/diagnostics/ -count=1
cd web && npm test
```
Expected: すべて成功。macOS の端末の挙動は 1 つも変わらないこと — `Terminal` が
nil でないときの経路には手を触れていないので、既存の diagnostics のテストは
すべてそのまま通るはずである。1 つでも落ちたら、それは触ってはいけないものを
触った証拠なので報告すること。

- [ ] **Step 6: コミット**

```bash
git add -A
git commit -m "Linux では端末を開かない

RunOutput は子プロセスの終了を待つ。macOS は /usr/bin/open を挟むので即座に
戻るが、Linux は端末エミュレータを直接起動するので、ウィンドウが閉じるまで
HTTP リクエストが開いたままになる。渡しているのはリクエストのコンテキスト
なので、タブを閉じれば SIGKILL が飛び、使用中の SSH セッションごと落ちる。

待たない起動口を作るか setsid を挟めば直るが、どちらも取らない。Linux では
コマンドを表示し、利用者が自分で実行する。

nil にするだけでは足りなかった。TerminalCommand は alias が安全なら無条件に
launchable を返し、TerminalOptions はランチャーが在庫を答えられないとき
「全部ある」と答えていた。後者の既定は「分からない」ための既定であって、
「無いと分かっている」ときのものではない。"
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
		// Terminal は設定しない。Linux では端末を開かず、コマンドを表示して
		// 利用者が実行する。理由はタスク 8 にある。nil は diagnostics が
		// 「端末が設定されていない」と報告する、支持された状態である。
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

- [ ] **Step 4: CI に macOS ジョブを足す**

計画はここで「ubuntu ジョブを足し、macOS のジョブはそのまま」と書いていたが、
前提が逆だった。`.github/workflows/ci.yml` の**全ジョブが `ubuntu-latest` で動いて
おり、macOS のジョブは存在しない**（`macos-14` を使うのは `release.yml` だけである）。

つまりこの作業以前は、ビルドタグが無かったおかげで ubuntu の CI が
`internal/platform/macos` も込みで検査していた。タグを付けた今、あの
600 行あまり — AppleScript の端末、launchd のログイン項目、ブラウザ — と
そのテストは、CI のどこでもコンパイルされない。これは今回の作業が持ち込んだ
後退であり、ubuntu ジョブをもう 1 つ足しても直らない。

既存の `go` ジョブ（`ubuntu-latest`）が Linux 側の検査としてそのまま働く。
足すのは macOS 側である。`go` ジョブの直後に、同じ形で置く:

```yaml
  macos:
    name: macOS
    runs-on: macos-14
    steps:
      - uses: actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09 # v5
      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6
        with:
          go-version-file: go.mod
          cache-dependency-path: go.sum

      # プラットフォーム層はビルドタグで分かれている。go ジョブは ubuntu で
      # 走るので、darwin のファイルはそこでは 1 行もコンパイルされない。この
      # ジョブが無ければ、macOS 側がコンパイルすら通らなくなったことに誰も
      # 気づかない。逆も同じで、Linux 側は go ジョブが見ている。
      - name: go vet
        run: go vet ./...

      - name: go build
        run: go build ./...

      - name: go test
        run: go test ./...
```

`gofmt` と `-race` は `go` ジョブが全ファイルに対して実行済みなので繰り返さない。
`gofmt` はビルドタグを見ないため、ubuntu 側で darwin のファイルも検査されている。
actions の SHA は既存ジョブからそのまま複製すること。

`runs-on` に `macos-14` を選ぶのは、`release.yml` が既にそれを使っており、
出荷しているものと同じ土俵で検査するためである。

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

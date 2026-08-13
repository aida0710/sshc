# 設定解決器の権威昇格 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `~/.ssh/config` を読んで「この alias で実際に何が使われるか」に答えるのを、`ssh -G` ではなくこのアプリケーション自身にする。

**Architecture:** `internal/effective` に `Resolve` を新設し、実機の OpenSSH との差分テストで育てる。育つまでは既存の 2 実装（`effective.Project` と `application.ComputeEffective`）を壊さず並走させ、通ってから呼び出し側を寄せ、最後に `ssh -G` の経路を消す。

**Tech Stack:** Go 1.26、既存の `internal/config`（パーサ）と `internal/effective`（射影）。新しい依存は無い。

## Global Constraints

- 設計は `docs/superpowers/specs/2026-08-13-config-resolver-authority-design.md`。矛盾したら spec が正。
- **既定値を持つのは 5 つだけ**: `HostName`（alias）、`User`（ローカル user 名）、`Port`（22）、`IdentityFile`（OpenSSH の既定の並び・累積）、`ProxyJump`（無し）。他は「書かれている値」をそのまま返す。
- **解決器は何も実行しない。** `Match exec` と `CanonicalizeHostname` は評価せず `Refusal` を返す。
- 部分的な答えを黙って返さない。接続に使う値が 1 つでも確定しないなら、その alias は解決できていない。
- パッケージの依存方向は `application → effective` の一方向。逆向きの import を作らない。
- コミット前に必ず `git diff --cached` で staged 内容そのものを読む（このリポジトリは別セッションが同時に編集していることがある）。
- ゲートは `go test ./...`、`go vet ./...`、`gofmt -l $(git ls-files '*.go')`。

---

### Task 1: 累積キーワードの表を共有し、`Project` の `Winner` を直す

`internal/application` にだけ正しい表があり、`internal/effective.Project` は一律の先勝ちなので、`IdentityFile` を 2 行書いた設定で 2 行目が「採用されない」と画面に出る。OpenSSH は両方を使う。

**Files:**
- Modify: `internal/effective/provenance.go`（表を追加し `Project` の `claimed` を直す）
- Modify: `internal/application/effective.go:13`（自前の表を消して `effective` のものを使う）
- Test: `internal/effective/provenance_test.go`

**Interfaces:**
- Produces: `effective.Cumulative(keyword string) bool` — 小文字化して判定する。

- [ ] **Step 1: 失敗するテストを書く**

```go
// OpenSSH は IdentityFile を積み上げる。最初の 1 つだけを勝たせると、
// 2 行目を書いた人には「効いていない」と表示されることになる。
func TestProjectKeepsEveryValueOfACumulativeKeyword(t *testing.T) {
	graph := newTestGraph(t, map[string]string{
		"config": "Host bastion\n\tIdentityFile ~/.ssh/a\n\tIdentityFile ~/.ssh/b\n\tUser first\n\tUser second\n",
	})

	projection := effective.Project(graph, "bastion")
	winners := map[string][]string{}
	for _, source := range projection.Sources {
		if source.Winner {
			key := strings.ToLower(source.Keyword)
			winners[key] = append(winners[key], source.Value)
		}
	}
	if len(winners["identityfile"]) != 2 {
		t.Errorf("identityfile winners = %#v, want both", winners["identityfile"])
	}
	if len(winners["user"]) != 1 || winners["user"][0] != "first" {
		t.Errorf("user winners = %#v, want only the first", winners["user"])
	}
}
```

- [ ] **Step 2: 失敗を確認する**

Run: `go test ./internal/effective -run TestProjectKeepsEveryValueOfACumulativeKeyword -v`
Expected: FAIL（`identityfile winners` が 1 件）

- [ ] **Step 3: 表を `effective` へ置き、`Project` を直す**

```go
// cumulativeKeywords は、最初の値だけを残すのではなく OpenSSH が積み上げる
// ディレクティブである。他のキーワードはすべて先勝ちに従う。
var cumulativeKeywords = map[string]bool{
	"identityfile": true, "certificatefile": true, "localforward": true,
	"remoteforward": true, "dynamicforward": true, "sendenv": true, "setenv": true,
}

// Cumulative は、そのキーワードが積み上がるかを報告する。
func Cumulative(keyword string) bool { return cumulativeKeywords[strings.ToLower(keyword)] }
```

`Project` の `directive` クロージャで `Winner: !claimed[keyword] || cumulativeKeywords[keyword]` にする。

- [ ] **Step 4: `application` 側の重複を消す**

`internal/application/effective.go` の `cumulativeKeywords` を削除し、`122` 行目の判定を `effective.Cumulative(lowered)` にする。

- [ ] **Step 5: 通ることを確認する**

Run: `go test ./internal/effective ./internal/application`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add internal/effective internal/application
git diff --cached   # 中身を読む
git commit
```

---

### Task 2: トークン展開

`%h` などは接続先が決まるまで確定しないので、いまは展開せず報告している。権威になるには展開できなければならない。

**Files:**
- Create: `internal/effective/tokens.go`
- Test: `internal/effective/tokens_test.go`

**Interfaces:**
- Produces:
  - `type LocalFacts struct { User, Home, Hostname string; UID string }`
  - `func ExpandTokens(value string, facts LocalFacts, target TokenTarget) (string, error)`
  - `type TokenTarget struct { Alias, HostName, Port, RemoteUser string }`
  - `var ErrUnknownToken = errors.New("that token cannot be expanded here")`

対応するトークン: `%%`（リテラルの `%`）、`%h`（HostName）、`%n`（元の alias）、`%p`（Port）、`%r`（リモート user）、`%u`（ローカル user）、`%d`（ホーム）、`%i`（uid）、`%L`（短いホスト名）、`%l`（FQDN）。それ以外は `ErrUnknownToken`。

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestExpandTokensReplacesWhatOpenSSHReplaces(t *testing.T) {
	facts := effective.LocalFacts{User: "aida", Home: "/home/aida", Hostname: "mac.local", UID: "501"}
	target := effective.TokenTarget{Alias: "bastion", HostName: "203.0.113.10", Port: "2222", RemoteUser: "ops"}

	for _, test := range []struct{ in, want string }{
		{"~/.ssh/%h.key", "~/.ssh/203.0.113.10.key"},
		{"%n", "bastion"},
		{"%r@%h:%p", "ops@203.0.113.10:2222"},
		{"%d/.ssh/id", "/home/aida/.ssh/id"},
		{"%u/%i", "aida/501"},
		{"100%%", "100%"},
	} {
		got, err := effective.ExpandTokens(test.in, facts, target)
		if err != nil || got != test.want {
			t.Errorf("ExpandTokens(%q) = %q, %v; want %q", test.in, got, err, test.want)
		}
	}

	if _, err := effective.ExpandTokens("%C", facts, target); !errors.Is(err, effective.ErrUnknownToken) {
		t.Errorf("%%C should not be expanded here")
	}
}
```

- [ ] **Step 2: 失敗を確認する**

Run: `go test ./internal/effective -run TestExpandTokens -v`
Expected: FAIL（`undefined: effective.ExpandTokens`）

- [ ] **Step 3: 実装する**

1 文字ずつ走査し、`%` の次の 1 文字で分岐する。末尾の単独 `%` は `ErrUnknownToken`。

- [ ] **Step 4: 通ることを確認してコミットする**

Run: `go test ./internal/effective`

---

### Task 3: `Match` ブロックの評価

パーサは既に `Block.Criteria []config.Criterion{Keyword, Argument, Negated}` を持っている。評価だけが無い。

**Files:**
- Create: `internal/effective/match.go`
- Test: `internal/effective/match_test.go`

**Interfaces:**
- Produces:
  - `type MatchContext struct { Alias, OriginalAlias, HostName, User, LocalUser string; Tags []string; Final, Canonical bool }`
  - `func MatchApplies(criteria []config.Criterion, ctx MatchContext) (bool, error)`
  - `var ErrMatchExec = errors.New("Match exec is not evaluated")`

規則: `all` は常に真。`host`／`originalhost`／`user`／`localuser`／`tagged` は `MatchPattern` でカンマ区切りのどれかに一致。`final`／`canonical` は文脈の真偽。`exec` は `ErrMatchExec`。`Negated` は結果を反転する。**すべての条件が真のときだけブロックが適用される。**

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestMatchAppliesEvaluatesEveryCriterionWithoutRunningAnything(t *testing.T) {
	ctx := effective.MatchContext{Alias: "db", HostName: "10.0.0.5", User: "ops", LocalUser: "aida"}

	for _, test := range []struct {
		name     string
		criteria []config.Criterion
		want     bool
	}{
		{"all", []config.Criterion{{Keyword: "all"}}, true},
		{"host matches", []config.Criterion{{Keyword: "host", Argument: "db"}}, true},
		{"host misses", []config.Criterion{{Keyword: "host", Argument: "web"}}, false},
		{"host list", []config.Criterion{{Keyword: "host", Argument: "web,db"}}, true},
		{"negated host", []config.Criterion{{Keyword: "host", Argument: "db", Negated: true}}, false},
		{"user and host", []config.Criterion{{Keyword: "host", Argument: "db"}, {Keyword: "user", Argument: "ops"}}, true},
		{"one criterion fails", []config.Criterion{{Keyword: "host", Argument: "db"}, {Keyword: "user", Argument: "root"}}, false},
		{"localuser", []config.Criterion{{Keyword: "localuser", Argument: "aida"}}, true},
	} {
		got, err := effective.MatchApplies(test.criteria, ctx)
		if err != nil || got != test.want {
			t.Errorf("%s: MatchApplies = %v, %v; want %v", test.name, got, err, test.want)
		}
	}

	_, err := effective.MatchApplies([]config.Criterion{{Keyword: "exec", Argument: "true"}}, ctx)
	if !errors.Is(err, effective.ErrMatchExec) {
		t.Errorf("Match exec = %v, want ErrMatchExec", err)
	}
}
```

- [ ] **Step 2: 失敗を確認する / Step 3: 実装する / Step 4: 通してコミットする**

Run: `go test ./internal/effective -run TestMatchApplies -v`

---

### Task 4: `Resolve` — 値を決める

**Files:**
- Create: `internal/effective/resolve.go`
- Test: `internal/effective/resolve_test.go`

**Interfaces:**
- Produces:
  - `type Refusal struct { Code, Path, Detail string; Line int }`
  - `const RefusalMatchExec = "match_exec"`, `RefusalCanonicalize = "canonicalize_hostname"`
  - `func Resolve(graph *config.Graph, alias string, facts LocalFacts) (Values, []Refusal)`

`Values` は既存の型（`Keywords []string`、`Entries map[string][]string`）をそのまま使う。

規則:
1. `walkLoadOrder` で読み込み順に走査する
2. `Host` ブロックは `blockApplies`、`Match` ブロックは `MatchApplies` で適用を決める
3. 値は先勝ち。ただし `Cumulative` なキーワードは積み上げる
4. 走査後、5 つの既定値を埋める（既に値があるものは触らない）
5. 値のトークンを `ExpandTokens` で展開する。`HostName` が決まってからでないと `%h` が展開できないので、**展開は走査の後に 1 回**行う
6. `Match exec` か `CanonicalizeHostname` に出会ったら `Refusal` を積み、`Values` は返さない（空を返す）

- [ ] **Step 1: 失敗するテストを書く**

```go
func TestResolveAnswersWithTheValuesTheConnectionUses(t *testing.T) {
	graph := newTestGraph(t, map[string]string{
		"config": "Host bastion\n\tHostName 203.0.113.10\n\tUser ops\n\nHost *\n\tPort 2222\n",
	})
	facts := effective.LocalFacts{User: "aida", Home: "/home/aida"}

	values, refusals := effective.Resolve(graph, "bastion", facts)
	if len(refusals) != 0 {
		t.Fatalf("refusals = %#v", refusals)
	}
	if values.First("hostname") != "203.0.113.10" || values.First("user") != "ops" {
		t.Errorf("values = %#v", values.Entries)
	}
	if values.First("port") != "2222" {
		t.Errorf("port = %q, want the wildcard block's value", values.First("port"))
	}
}

func TestResolveFillsTheDefaultsItOwns(t *testing.T) {
	graph := newTestGraph(t, map[string]string{"config": "Host bare\n\tCompression yes\n"})
	values, _ := effective.Resolve(graph, "bare", effective.LocalFacts{User: "aida", Home: "/home/aida"})

	if values.First("hostname") != "bare" || values.First("user") != "aida" || values.First("port") != "22" {
		t.Errorf("defaults = %#v", values.Entries)
	}
	// 書かれている他のキーワードはそのまま返る。既定値は持たない。
	if values.First("compression") != "yes" {
		t.Errorf("compression = %q", values.First("compression"))
	}
	if _, present := values.Entries["serveraliveinterval"]; present {
		t.Errorf("a keyword nobody wrote must not appear")
	}
}

func TestResolveRefusesWhatItWillNotEvaluate(t *testing.T) {
	graph := newTestGraph(t, map[string]string{
		"config": "Match exec \"true\"\n\tUser hidden\n\nHost bastion\n\tUser ops\n",
	})
	values, refusals := effective.Resolve(graph, "bastion", effective.LocalFacts{User: "aida"})

	if len(refusals) != 1 || refusals[0].Code != effective.RefusalMatchExec {
		t.Fatalf("refusals = %#v", refusals)
	}
	if len(values.Entries) != 0 {
		t.Errorf("a refused configuration must not carry partial values: %#v", values.Entries)
	}
}
```

- [ ] **Step 2〜4: 失敗を確認し、実装し、通してコミットする**

---

### Task 5: 差分テストを `Resolve` に向け、フィクスチャを増やす

**Files:**
- Modify: `internal/effective/differential_test.go`

`TestProjectionMatchesInstalledOpenSSH` は `Project` の射影を見ている。`Resolve` の `Values` を実機の `ssh -G` と突き合わせる形へ変え、フィクスチャを spec の完成条件まで増やす。

追加するフィクスチャ: `Match host`、`Match user`、`Match localuser`、`Match final`、トークン展開（`%h` `%r` `%d`）、`IdentityFile` を複数、`SetEnv` を複数、既定値（何も書かない alias）。

- [ ] **Step 1〜3: フィクスチャを 1 つずつ足し、落ちたら `Resolve` を直す**

Run: `go test ./internal/effective -run TestProjectionMatchesInstalledOpenSSH -v`

- [ ] **Step 4: Linux でも確かめる**

```sh
docker run --rm -v "$PWD":/src -w /src golang:1.26 sh -c "go test ./internal/effective -count=1"
```

---

### Task 6: 呼び出し側を `Resolve` へ寄せる

**Files:**
- Modify: `internal/application/effective.go`（`ComputeEffective` を `Resolve` の上に載せ替える）
- Modify: `internal/diagnostics/service.go:179`
- Test: 既存の `internal/application` と `internal/httpserver` のスイート

`ComputeEffective` の返す `Effective` の形は変えない（API の契約なので）。中身の決定を `Resolve` に任せる。

- [ ] **Step 1: `ComputeEffective` を `Resolve` の呼び出しに置き換える**
- [ ] **Step 2: `go test ./...` を通す**
- [ ] **Step 3: コミット**

---

### Task 7: 画面から「近似」を外す

**Files:**
- Modify: `internal/application/effective.go`（`Approximate: true` と `NoticeExplainedValuesOnly`）
- Modify: `web/src/i18n/messages/{en,ja}.ts`（`explained_values_only` の文言）
- Modify: 該当のテストと e2e

答えが確定するようになったので、「値の出所説明のみ」という但し書きを外す。`Refusal` があるときだけ、その理由を出す。

- [ ] **Step 1〜3: 変更し、`npm test` と `make e2e` を通し、コミットする**

---

### Task 8: `ssh -G` の経路を削除する

**Files:**
- Delete: `internal/effective/evaluate.go` と `evaluate_test.go`、`fuzz_test.go` の `FuzzParseValues`
- Modify: `api/openapi.yaml`（`/api/v1/diagnostics/effective`）、`internal/httpserver/diagnostics.go`、`internal/diagnostics/service.go`
- Modify: `Makefile`（`FUZZ_TARGETS` から `FuzzParseValues` を外す）
- Modify: `web/src`（確認ダイアログと呼び出し）
- Modify: `README.md`

**差分テストは残す。** テストの中でだけ `ssh -G` を回し、`Resolve` の答えと突き合わせる。製品からは消える。

- [ ] **Step 1: 削除する**
- [ ] **Step 2: `make verify-generated` と全ゲートを通す**
- [ ] **Step 3: README を書き換える**（失ったものと得たものを両方書く）
- [ ] **Step 4: コミット**

---

## 自己レビュー

**spec の網羅**: 決定事項 1（既定値 5 つ）→ Task 4。決定事項 2（`Match exec` 拒否）→ Task 3・4。決定事項 3（`ssh -G` 削除）→ Task 8。決定事項 4（2 実装の統合）→ Task 1・6。足りないもの A〜E → Task 3（A）、2（B）、1（C）、4（D）、4（E は `Refusal`）。完成条件 → Task 5。

**型の一貫性**: `LocalFacts` は Task 2 で定義し 4 で使う。`Values` は既存。`Refusal` は Task 4 で定義し 7 で使う。`Cumulative` は Task 1 で定義し 4 で使う。`MatchContext` と `MatchApplies` は Task 3 で定義し 4 で使う。

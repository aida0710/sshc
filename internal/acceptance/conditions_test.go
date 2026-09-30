package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type proofKind int

const (
	proofGoTest proofKind = iota
	proofVitest
	proofPlaywright
	proofCommand
	proofManual
)

func (k proofKind) String() string {
	switch k {
	case proofGoTest:
		return "go"
	case proofVitest:
		return "vitest"
	case proofPlaywright:
		return "e2e"
	case proofCommand:
		return "make"
	case proofManual:
		return "manual"
	default:
		return "?"
	}
}

type proof struct {
	Kind      proofKind
	Reference string
}

// verdict は、ある 1 つの condition について automation が実際どこまで到達するかを示す。
type verdict int

const (
	// verdictAutomated: CI が走らせる automation だけで condition を端から端まで
	// 証明する。CI が走らせない e2e や make fuzz に頼る condition はこれにしない。
	verdictAutomated verdict = iota
	// verdictPartial: automation は条件の一部までを証明し、残りは人が確かめる。
	// 残りは、automation が越えてはならない境界の先（Manual が指定する）か、
	// CI が走らせない検査である。Gap には欠落が要る。
	verdictPartial
	// verdictConditional: automation は任意の capability が
	// あるときのみ証明し、なければ unproven として記録する。
	verdictConditional
)

func (v verdict) String() string {
	switch v {
	case verdictAutomated:
		return "HOLDS by automation"
	case verdictPartial:
		return "PARTIAL: automation covers part of it; a person checks the rest"
	case verdictConditional:
		return "CONDITIONAL: proven only when the capability is present"
	default:
		return "?"
	}
}

type completionCondition struct {
	Number int
	// Text は design §12 の逐語そのもの、行 13 のみ §10.1 の逐語である。
	Text string
	// Automated は、機械が検査するものすべてを指定する。
	Automated []proof
	// Manual は、automation が行ってはならない部分を指定する。
	Manual []proof
	// Verdict は、上の 2 つのリストを正直に読んだ結論である。
	Verdict verdict
	// Gap は、証明されていないものを率直に述べる。verdict が
	// verdictAutomated である場合を除き必須である。
	Gap string
}

// e2eRunByHand は、CI が走らせない e2e に頼る condition の Gap を結ぶ文である。
// CI は e2e のうち accessibility.spec.ts だけを走らせる（.github/workflows/ci.yml）。
const e2eRunByHand = "e2e の部分は、人が make e2e を走らせたときにだけ確かめる。" +
	"CI が走らせる e2e は e2e/accessibility.spec.ts だけである。"

func completionConditions() []completionCondition {
	return []completionCondition{
		{
			Number:  1,
			Text:    "既存 fixture を無変更で読み書きして byte-for-byte 一致する",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofGoTest, "FuzzParseRendersOriginalBytes"},
				{proofGoTest, "FuzzParseKnownHostsRoundTrip"},
				{proofGoTest, "TestResolveEditAndCommitPreservesEveryOtherByte"},
				{proofCommand, "fuzz"},
				{proofPlaywright, "edits a host through the form and writes only the line that changed"},
			},
			Gap: "fuzz の seed と、編集して保存してもほかのバイトを変えない検査は go test に含まれ、CI が走らせる。" +
				"seed 以外の入力を試すのは、人が make fuzz を走らせたときだけである。" +
				"画面のフォームから保存して変えた行だけが書かれることは、e2e だけが証明する。" + e2eRunByHand,
		},
		{
			Number:  2,
			Text:    "一般的な項目はフォーム、すべての項目は Raw で編集できる",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofPlaywright, "edits a host through the form and writes only the line that changed"},
				{proofPlaywright, "edits the same host through Raw and keeps every other byte"},
				{proofPlaywright, "shows the Include hierarchy and edits an included file"},
			},
			Gap: "フォームと Raw での編集を証明するのは e2e だけで、CI が走らせる証明は無い。" + e2eRunByHand,
		},
		{
			Number:  3,
			Text:    "コメント、未知ディレクティブ、Include 構造を保持する",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofGoTest, "FuzzParseRendersOriginalBytes"},
				{proofGoTest, "FuzzExpandIncludePattern"},
				{proofPlaywright, "shows the Include hierarchy and edits an included file"},
				{proofGoTest, "TestAnAliasOpenSSHWouldAcceptIsStillRefusedForEveryExternalEffect"},
			},
			Gap: "コメントと未知ディレクティブの保持、Include パターンの展開は go test（fuzz の seed を含む）で CI が確かめる。" +
				"Include の階層を画面に表示し、読み込まれたファイルを編集できることは、e2e だけが証明する。" + e2eRunByHand,
		},
		{
			Number: 4,
			Text:   "Include 階層、単一プライマリグループ、親子継承が機能する",
			Automated: []proof{
				{proofGoTest, "TestCompileGroupsPutsChildrenBeforeParentsAndInheritsMembers"},
				{proofGoTest, "TestCompileGroupsRendersParsableLosslessConfiguration"},
				{proofGoTest, "TestAGroupCanNeverBeItsOwnAncestor"},
				{proofGoTest, "TestPlanRegionEmitsOneIncludePerGroupChildFirst"},
				{proofGoTest, "TestPlanRegionPutsTheRegionAboveEveryHostBlock"},
				{proofGoTest, "TestPlanRegionMovesARegionThatSitsInsideAHostBlock"},
				{proofGoTest, "TestHostEntryGroupComesFromTheDirectoryNotFromMetadata"},
				{proofGoTest, "TestRouteTableMatchesTheOpenAPIContract"},
				{proofPlaywright, "shows the Include hierarchy and edits an included file"},
				{proofPlaywright, "declares a group in the entry file and moves a connection into it"},
				{proofPlaywright, "gives a nested group its own Include line, deepest first"},
			},
			Verdict: verdictPartial,
			Gap: "グループの展開、Include の順序、親子の継承は go test で CI が確かめる。" +
				"画面でグループを宣言して接続を移し、入れ子のグループに Include の行を書くことは、e2e だけが証明する。" + e2eRunByHand,
		},
		{
			Number:  5,
			Text:    "多段 ProxyJump と値の出所を表示できる",
			Verdict: verdictConditional,
			Automated: []proof{
				{proofGoTest, "TestResolveMatchesInstalledOpenSSH"},
				{proofGoTest, "FuzzResolve"},
			},
			Gap: "the differential proof runs the installed OpenSSH. On a machine " +
				"without it TestResolveMatchesInstalledOpenSSH skips, and this " +
				"condition is then unproven rather than passing quietly.",
		},
		{
			Number:  6,
			Text:    "鍵生成、公開鍵コピー、秘密鍵 reveal、agent 登録、隔離、復元が機能する",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofPlaywright, "lists generated keys and reveals one only after an explicit confirmation"},
				{proofGoTest, "TestGenerateWritesAnEncryptedPairThroughATransaction"},
				{proofGoTest, "TestTrashMovesTheWholeKeyPairAndKeepsItsPermissions"},
				{proofGoTest, "TestRegisterSendsTheKeyContentsNamedByItsPathAndThePassphraseToTheAgentOnly"},
				{proofGoTest, "TestEveryGuardedRouteRefusesAMissingWrongOrExpiredToken"},
				{proofGoTest, "TestNoResponseCarriesASecretItIsNotEntitledTo"},
			},
			Manual: []proof{{proofManual, "M3. 実 ssh-agent"}},
			Gap: "鍵の生成、隔離、agent への登録と、応答にシークレットを載せないことは go test で CI が確かめる。" +
				"agent への登録はプロセス内の keyring と本物のプロトコルでやり取りするので、やり取りの形式もここで証明される。" +
				"生成した鍵を画面の一覧に出し、明示の確認のあとにだけ秘密鍵を表示する（reveal）ことは、e2e だけが証明する。" +
				e2eRunByHand +
				"利用者自身の ssh-agent が同じように振る舞うことと、その間に ssh-add のプロセスが現れないことは、手動の M3 で確かめる。",
		},
		{
			Number:  7,
			Text:    "config 変更前に差分、保存前にバックアップを確認できる",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofPlaywright, "shows a save preview diff of exactly what was written"},
				{proofPlaywright, "records a change in history and restores the previous bytes"},
				{proofGoTest, "TestCommitWritesEveryChangeAndRecordsHistory"},
			},
			Gap: "変更を書いて履歴に残すことは go test で CI が確かめる。" +
				"保存前に差分を画面に表示し、履歴から前のバイト列へ戻せることは、e2e だけが証明する。" + e2eRunByHand,
		},
		{
			Number:  8,
			Text:    "外部変更または部分失敗時に既存設定を暗黙に変更しない",
			Verdict: verdictPartial,
			Automated: []proof{
				// 外部変更、端から端まで、かつトランザクション境界において。
				{proofPlaywright, "refuses a save whose base is stale and shows the three-way conflict"},
				{proofGoTest, "TestCommitRejectsExternalChangesWithThreeWayData"},
				// 部分的失敗: staging、rename、rollback それぞれにテストがある。
				{proofGoTest, "TestCommitFailureWhileStagingLeavesEveryFileUntouched"},
				{proofGoTest, "TestCommitLeavesRecoverableJournalWhenRenameFails"},
				{proofGoTest, "TestRollbackRestoresEveryCommittedFile"},
				{proofGoTest, "TestPendingDescribesTheInterruptedTransaction"},
				{proofGoTest, "TestNoRouteWritesOutsideTheWorkspaceOrThroughASymbolicLink"},
			},
			Gap: "外部変更を commit の境界で three-way のデータとともに断ることは、go test で CI が確かめる。" +
				"古い base からの保存を画面で断り、three-way の衝突を表示することは、e2e だけが証明する。" +
				e2eRunByHand +
				"部分失敗は、commit の途中でプロセスを止めるのではなく、storage の層に失敗を差し込んで証明している。" +
				"staging の書き込みと rename の間の電源断や SIGKILL は、journal のテストからの推論で扱い、観察はしていない。" +
				"symlink の側には独立した関門が2つあるので、1層だけの退行は route の検査では表に出ない。" +
				"TestTheWorkspaceGuardRefusesTraversalAndSymlinksWithoutTheHTTPLayer はそのためにある。",
		},
		{
			Number:  9,
			Text:    "接続テスト、埋め込みターミナル、Known Hosts、公開鍵登録が明示操作で機能する",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofGoTest, "TestEveryGuardedRouteRefusesAMissingWrongOrExpiredToken"},
				{proofGoTest, "TestARealPseudoTerminalCarriesTheOutputAndTheExitStatus"},
				{proofGoTest, "TestRemoteRegistrationNeverInterpolatesInputIntoTheRemoteShell"},
				{proofPlaywright, "opens a local shell, runs a command and shows its output"},
				{proofPlaywright, "lists the known_hosts entries and deletes one through a confirmation"},
				{proofPlaywright, "shows the alias, effective user, fingerprint and the exact line before registering"},
			},
			Manual: []proof{
				{proofManual, "M1. 実リモートホストへの接続テスト"},
				{proofManual, "M2. 実 `authorized_keys` への公開鍵登録"},
			},
			Gap: "SSH はプロセス内で通信するので、internal/sshclient は 127.0.0.1 に立てたサーバーと" +
				"本物のハンドシェイクを行い、認証・転送・ホスト鍵・リモート実行を端から端まで見る。" +
				"これと本物の PTY の検査は go test で CI が確かめる。" +
				"画面でローカルシェルを開いてキーを打ち、出力が表示されること、known_hosts の行を確認のあとに消すこと、" +
				"公開鍵を登録する前に alias、実効ユーザー、フィンガープリント、書き込む行を表示することは、e2e だけが証明する。" +
				e2eRunByHand +
				"実リモートに触れる部分は自動化しない。本物のサーバーが認証を通すことと、" +
				"その authorized_keys に行が現れることは、それぞれ手動の M1 と M2 で確かめる。",
		},
		{
			Number:  10,
			Text:    "localhost API が token、Host、Origin、Fetch Metadata で保護される",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofGoTest, "TestEveryAPIRouteRefusesTheWrongHostOriginAndFetchSite"},
				{proofGoTest, "TestEveryAPIRouteExceptBootstrapRequiresASession"},
				{proofGoTest, "TestBootstrapTokenIsSingleUse"},
				{proofGoTest, "TestServerRefusesEveryListenerThatIsNotUnmappedLoopbackIPv4"},
				{proofGoTest, "TestEveryAPIResponseIsNoStoreAndCarriesTheExactPolicy"},
				{proofGoTest, "TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM"},
				{proofPlaywright, "exchanges the fragment for a session and removes it from the address bar"},
				{proofPlaywright, "enforces the content security policy in the browser, not only in the header"},
			},
			Gap: "API の保護（token、Host、Origin、Fetch Metadata）は go test で CI が確かめる。" +
				"ブラウザが URL の fragment を session に換えてアドレスバーから消すこと、CSP をブラウザが実際に適用することは、e2e だけが証明する。" +
				e2eRunByHand,
		},
		{
			Number:  11,
			Text:    "危険ディレクティブを暗黙実行しない",
			Verdict: verdictPartial,
			Automated: []proof{
				// 設定を読むことは、もう何も起動しない。Match exec は評価せず拒む。
				{proofGoTest, "TestResolveRefusesWhatItWillNotEvaluate"},
				// 接続前の検証。
				{proofGoTest, "TestEveryGuardedRouteRefusesAMissingWrongOrExpiredToken"},
				{proofGoTest, "TestNoRouteEverLetsAHostileAliasReachAnExternalEffect"},
				{proofGoTest, "TestTheRemoteSeamRefusesAHostileAliasWithoutTheHTTPGuard"},
			},
			Manual: []proof{{proofManual, "M1. 実リモートホストへの接続テスト"}},
			Gap: "アプリケーションの判断だけで外部プログラムを起動しない。" +
				"Match exec は解決の時点で断り、LocalCommand も KnownHostsCommand も " +
				"このクライアントに機能として無い。ProxyCommand は明示された設定に従って起動する。" +
				"利用者が指定したコマンドを拒否すると、その接続先を扱えなくなる。" +
				"暗黙には起動せず、コマンドは接続のたびに " +
				"端末へ 1 行出る。RemoteCommand は設定に書かれたコマンドをリモートで" +
				"走らせる。どちらも利用者が書いたとおりのことであり、暗黙ではない。" +
				"自動化が届かないのは、実リモートがそれをどう扱うかだけで、それが M1 " +
				"である。alias の関門は三層あるので、どれか一層だけを外しても route の" +
				"検査は緑のままである点にも注意すること。",
		},
		{
			Number:  12,
			Text:    "バックエンド、フロントエンド、セキュリティ、race、E2E テストが成功する",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofCommand, "test"},
				{proofCommand, "fuzz"},
				{proofCommand, "e2e"},
				{proofCommand, "verify-generated"},
				{proofGoTest, "TestMakefileFuzzTargetsCoverEveryFuzzFunction"},
				{proofGoTest, "TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM"},
			},
			Gap: "バックエンド、フロントエンド、race、生成物の一致は CI が走らせる。" +
				"セキュリティのうち、API の保護やログへのシークレットの混入の検査は go test に含まれる。" +
				"依存の脆弱性の検査（govulncheck、npm audit）は make のターゲットに無く、CI の security ジョブだけが走らせる。" +
				"E2E と fuzz の成功は、人が make e2e と make fuzz を走らせたときにだけ確かめる。" +
				"CI が走らせるのは e2e/accessibility.spec.ts と、通常の go test が実行する fuzz の seed だけである。",
		},
		{
			Number:  13,
			Text:    "自動テストは実際の ~/.ssh、Keychain、ssh-agent、Terminal、実サーバーを使用しない（§10.1）",
			Verdict: verdictPartial,
			Automated: []proof{
				{proofGoTest, "TestHarnessStartsTheProductionServerAgainstAnIsolatedHome"},
				{proofGoTest, "TestNoTestOnlyPackageReachesTheShippedBinary"},
				{proofGoTest, "TestNoLogLineCarriesASecret"},
				{proofGoTest, "TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM"},
			},
			Manual: []proof{{proofManual, "M5. 実 `~/.ssh` での読み取り専用リハーサル"}},
			Gap: "no automated check forbids a future test from reading the real home: " +
				"the rule is enforced by review and by the fact that nothing under " +
				"internal/ may read $HOME. That a realistic personal configuration " +
				"survives being browsed is manual test M5.",
		},
	}
}

func TestDesignCompletionConditionsNameExistingProofsAndStateTheirGaps(t *testing.T) {
	repository := filepath.Join("..", "..")
	sources := collectSources(t, repository)

	for _, condition := range completionConditions() {
		t.Run(fmt.Sprintf("condition_%02d", condition.Number), func(t *testing.T) {
			if len(condition.Automated) == 0 {
				t.Fatalf("condition %d names no automated proof", condition.Number)
			}
			for _, item := range append(append([]proof(nil), condition.Automated...), condition.Manual...) {
				if !proofExists(sources, item) {
					t.Errorf("condition %d names %s proof %q, which no longer exists",
						condition.Number, item.Kind, item.Reference)
				}
			}
			// automation が完了できない condition はそう述べねばならず、
			// 完了したと主張する condition は手動の手順を一切指定してはならない。
			if condition.Verdict != verdictAutomated && condition.Gap == "" {
				t.Errorf("condition %d is not fully automated but states no gap", condition.Number)
			}
			if condition.Verdict == verdictAutomated && len(condition.Manual) > 0 {
				t.Errorf("condition %d claims full automation but names a manual step", condition.Number)
			}
			if condition.Verdict == verdictAutomated && condition.Gap != "" {
				t.Errorf("condition %d claims full automation but states a gap", condition.Number)
			}
			// CI が走らせない証明に頼る condition は、CI の結果だけでは成立を
			// 確かめられないので、完了したとは主張できない。
			if condition.Verdict == verdictAutomated {
				for _, item := range condition.Automated {
					if !runsInCI(sources, item) {
						t.Errorf("condition %d claims full automation, but CI does not run its %s proof %q",
							condition.Number, item.Kind, item.Reference)
					}
				}
			}
			// CI が走らせない e2e に頼る condition は、人が make e2e を走らせないと
			// 確かめられないことを Gap に書く。書かないと、監査の出力が人の確かめる
			// 部分から e2e を落とす。
			if reliesOnE2EOutsideCI(sources, condition) && !strings.Contains(condition.Gap, e2eRunByHand) {
				t.Errorf("condition %d relies on e2e that CI does not run, but its gap does not include e2eRunByHand",
					condition.Number)
			}

			t.Logf("\n%2d  %s\n    %s%s", condition.Number, condition.Text, condition.Verdict,
				gapLine(condition.Gap))
		})
	}
}

func gapLine(gap string) string {
	if gap == "" {
		return ""
	}
	return "\n    gap: " + gap
}

// TestCompletionAuditCountsWhatItClaims は、要約を正直に保つ。
//
// この監査が役立つのは、報告する condition の数が design
// §12 が実際に挙げる数と一致し、かつ verdict の内訳が
// 読み手に数えさせるのではなく明記されている場合に限る。
func TestCompletionAuditCountsWhatItClaims(t *testing.T) {
	conditions := completionConditions()
	// design §12 の 12 個に、行 13 として §10.1 の隔離規則を加えたもの。
	if len(conditions) != 13 {
		t.Fatalf("the audit lists %d conditions, want 13", len(conditions))
	}
	seen := map[int]bool{}
	counts := map[verdict]int{}
	for _, condition := range conditions {
		if seen[condition.Number] {
			t.Errorf("condition %d is listed twice", condition.Number)
		}
		seen[condition.Number] = true
		counts[condition.Verdict]++
	}
	for number := 1; number <= 13; number++ {
		if !seen[number] {
			t.Errorf("condition %d is missing from the audit", number)
		}
	}
	t.Logf("verdicts: %d hold by automation, %d partial, %d conditional",
		counts[verdictAutomated], counts[verdictPartial], counts[verdictConditional])

	// docs/design.md は内訳を数で書く。verdict を変えたら本文の数も同じ変更で直す。
	summary := completionSummary(len(conditions), counts)
	design := mustReadText(t, filepath.Join("..", "..", "docs", "design.md"))
	if !strings.Contains(design, summary) {
		t.Errorf("docs/design.md does not state the current breakdown; write:\n%s", summary)
	}
}

// completionSummary は、docs/design.md が完成条件の内訳を述べる文である。
func completionSummary(total int, counts map[verdict]int) string {
	automated := fmt.Sprintf("%d行が自動テストだけで成立し", counts[verdictAutomated])
	if counts[verdictAutomated] == 0 {
		automated = "自動テストだけで成立する行はなく"
	}
	return fmt.Sprintf("%d行のうち%s、%d行は自動テストが届かない部分を人が確かめ、%d行はOpenSSHが入っている場合にだけ証明されます。",
		total, automated, counts[verdictPartial], counts[verdictConditional])
}

// makeTargetsCIDoesNotRun は、CI（.github/workflows/ci.yml）が走らせない make の
// ターゲットである。e2e は時間がかかるので CI は accessibility.spec.ts だけを走らせ、
// fuzz は通常の go test が seed だけを実行する。
var makeTargetsCIDoesNotRun = map[string]bool{"e2e": true, "fuzz": true}

// ciPlaywrightSpecPattern は、CI の workflow が名前で指定して走らせる Playwright の
// spec を拾う。
var ciPlaywrightSpecPattern = regexp.MustCompile(`e2e/([A-Za-z0-9_.-]+\.spec\.ts)`)

// runsInCI は、その証明を CI が走らせるかを返す。go test と vitest は CI がすべて
// 走らせる。manual の手順は CI の外にある。
func runsInCI(sources sourceIndex, item proof) bool {
	switch item.Kind {
	case proofPlaywright:
		return strings.Contains(sources.playwrightInCI, item.Reference)
	case proofCommand:
		return !makeTargetsCIDoesNotRun[item.Reference]
	case proofManual:
		return false
	default:
		return true
	}
}

// reliesOnE2EOutsideCI は、condition の証明に CI が走らせない Playwright の spec が
// 含まれるかを返す。
func reliesOnE2EOutsideCI(sources sourceIndex, condition completionCondition) bool {
	for _, item := range condition.Automated {
		if item.Kind == proofPlaywright && !runsInCI(sources, item) {
			return true
		}
	}
	return false
}

type sourceIndex struct {
	goTests    string
	vitest     string
	playwright string
	// playwrightInCI は、CI が走らせる spec だけの中身である。
	playwrightInCI string
	makefile       string
	manual         string
}

// ciPlaywrightSpecs は、CI の workflow が走らせる Playwright の spec のファイル名を返す。
func ciPlaywrightSpecs(t testing.TB, repository string) map[string]bool {
	t.Helper()
	workflow := mustReadText(t, filepath.Join(repository, ".github", "workflows", "ci.yml"))
	specs := map[string]bool{}
	for _, match := range ciPlaywrightSpecPattern.FindAllStringSubmatch(workflow, -1) {
		specs[match[1]] = true
	}
	return specs
}

func collectSources(t testing.TB, repository string) sourceIndex {
	t.Helper()
	index := sourceIndex{
		makefile: mustReadText(t, filepath.Join(repository, "Makefile")),
		manual:   mustReadText(t, filepath.Join(repository, "docs", "manual-acceptance.md")),
	}
	specsInCI := ciPlaywrightSpecs(t, repository)
	var goTests, vitest, playwright, playwrightInCI strings.Builder
	err := filepath.WalkDir(repository, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "bin", ".claude", ".worktrees", "dist", ".playwright-browsers":
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		switch {
		case strings.HasSuffix(name, "_test.go"):
			goTests.WriteString(mustReadText(t, path))
		case strings.HasSuffix(name, ".spec.ts"):
			spec := mustReadText(t, path)
			playwright.WriteString(spec)
			if specsInCI[name] {
				playwrightInCI.WriteString(spec)
			}
		case strings.HasSuffix(name, ".test.ts"), strings.HasSuffix(name, ".test.tsx"):
			vitest.WriteString(mustReadText(t, path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	index.goTests = goTests.String()
	index.vitest = vitest.String()
	index.playwright = playwright.String()
	index.playwrightInCI = playwrightInCI.String()
	if index.goTests == "" || index.playwright == "" {
		t.Fatal("the walk collected no Go tests or no Playwright specs; the audit is not looking in the right place")
	}
	return index
}

func mustReadText(t testing.TB, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}

func proofExists(sources sourceIndex, item proof) bool {
	switch item.Kind {
	case proofGoTest:
		return strings.Contains(sources.goTests, "func "+item.Reference+"(")
	case proofVitest:
		return strings.Contains(sources.vitest, item.Reference)
	case proofPlaywright:
		return strings.Contains(sources.playwright, item.Reference)
	case proofCommand:
		return strings.Contains(sources.makefile, "\n"+item.Reference+":")
	case proofManual:
		return strings.Contains(sources.manual, item.Reference)
	default:
		return false
	}
}

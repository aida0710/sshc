package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
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
	// verdictAutomated: automation が condition を端から端まで証明する。
	verdictAutomated verdict = iota
	// verdictPartial: automation は越えてはならない境界まで
	// 証明し、残りは Manual が指定する。Gap には欠落が要る。
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
		return "PARTIAL: automation stops at a boundary it must not cross"
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

func completionConditions() []completionCondition {
	return []completionCondition{
		{
			Number:  1,
			Text:    "既存 fixture を無変更で読み書きして byte-for-byte 一致する",
			Verdict: verdictAutomated,
			Automated: []proof{
				{proofGoTest, "FuzzParseRendersOriginalBytes"},
				{proofGoTest, "FuzzParseKnownHostsRoundTrip"},
				{proofGoTest, "TestResolveEditAndCommitPreservesEveryOtherByte"},
				{proofCommand, "fuzz"},
				{proofPlaywright, "edits a host through the form and writes only the line that changed"},
			},
		},
		{
			Number:  2,
			Text:    "一般的な項目はフォーム、すべての項目は Raw で編集できる",
			Verdict: verdictAutomated,
			Automated: []proof{
				{proofPlaywright, "edits a host through the form and writes only the line that changed"},
				{proofPlaywright, "edits the same host through Raw and keeps every other byte"},
				{proofPlaywright, "shows the Include hierarchy and edits an included file"},
			},
		},
		{
			Number:  3,
			Text:    "コメント、未知ディレクティブ、Include 構造を保持する",
			Verdict: verdictAutomated,
			Automated: []proof{
				{proofGoTest, "FuzzParseRendersOriginalBytes"},
				{proofGoTest, "FuzzExpandIncludePattern"},
				{proofPlaywright, "shows the Include hierarchy and edits an included file"},
				{proofGoTest, "TestAnAliasOpenSSHWouldAcceptIsStillRefusedForEveryExternalEffect"},
			},
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
			Verdict: verdictAutomated,
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
				{proofGoTest, "TestRegisterSendsTheKeyPathAndPassphraseToTheAgentOnly"},
				{proofGoTest, "TestEveryGuardedRouteRefusesAMissingWrongOrExpiredToken"},
				{proofGoTest, "TestNoResponseCarriesASecretItIsNotEntitledTo"},
			},
			Manual: []proof{{proofManual, "M3. 実 ssh-agent"}},
			Gap: "agent registration speaks the real protocol to an in-process keyring, " +
				"so the wire format is proven here. What is not proven is that the " +
				"user's own agent behaves the same way, and that no ssh-add process " +
				"appears while it happens: that is manual test M3.",
		},
		{
			Number:  7,
			Text:    "config 変更前に差分、保存前にバックアップを確認できる",
			Verdict: verdictAutomated,
			Automated: []proof{
				{proofPlaywright, "shows a save preview diff of exactly what was written"},
				{proofPlaywright, "records a change in history and restores the previous bytes"},
				{proofGoTest, "TestCommitWritesEveryChangeAndRecordsHistory"},
			},
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
			Gap: "partial failure is proven by injecting a failure into the storage " +
				"layer, not by killing the process mid-commit. A power loss or SIGKILL " +
				"between the staging write and the rename is covered by inference from " +
				"the journal tests, not by observation. The symlink half also has two " +
				"independent guards, so a single-layer regression would not surface in " +
				"the route-level test; TestTheWorkspaceGuardRefusesTraversalAndSymlinksWithoutTheHTTPLayer " +
				"exists because of that.",
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
			Gap: "端末を開くところまでは自動化された。SSH はプロセス内で通信するので、" +
				"internal/sshclient は 127.0.0.1 に立てたサーバーと本物の握手を行い、" +
				"認証・転送・ホスト鍵・リモート実行を端から端まで見る。end-to-end は" +
				"ローカルシェルを本物の PTY で起動し、キーを打って出力が画面に出る" +
				"ところまでを見る。残っているのは実リモートに触れる二つである。" +
				"本物のサーバーが認証を通すこと、その authorized_keys に行が現れることで、" +
				"それぞれ M1 と M2 に対応する。",
		},
		{
			Number:  10,
			Text:    "localhost API が token、Host、Origin、Fetch Metadata で保護される",
			Verdict: verdictAutomated,
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
			Verdict: verdictAutomated,
			Automated: []proof{
				{proofCommand, "test"},
				{proofCommand, "fuzz"},
				{proofCommand, "e2e"},
				{proofCommand, "verify-generated"},
				{proofGoTest, "TestMakefileFuzzTargetsCoverEveryFuzzFunction"},
				{proofGoTest, "TestBuiltBinaryServesTheEmbeddedUIAndStopsOnSIGTERM"},
			},
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

func TestDesignCompletionConditions(t *testing.T) {
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
}

type sourceIndex struct {
	goTests    string
	vitest     string
	playwright string
	makefile   string
	manual     string
}

func collectSources(t testing.TB, repository string) sourceIndex {
	t.Helper()
	index := sourceIndex{
		makefile: mustReadText(t, filepath.Join(repository, "Makefile")),
		manual:   mustReadText(t, filepath.Join(repository, "docs", "manual-acceptance.md")),
	}
	var goTests, vitest, playwright strings.Builder
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
			playwright.WriteString(mustReadText(t, path))
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

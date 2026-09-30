package session

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"
	"time"
)

func TestBootstrapCreatesAuthenticatedSessionOnce(t *testing.T) {
	random := bytes.NewReader(bytes.Repeat([]byte{0x42}, 96))
	manager, bootstrap, err := NewManager(random)
	if err != nil {
		t.Fatal(err)
	}
	if len(bootstrap) != 43 {
		t.Fatalf("bootstrap length = %d", len(bootstrap))
	}

	credentials, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	if ok := acceptsRequest(manager, credentials.SessionID); !ok {
		t.Fatal("new session was not authenticated")
	}
	if !manager.VerifyCSRF(credentials.SessionID, credentials.CSRFToken) {
		t.Fatal("csrf token was rejected")
	}
	if _, _, err := manager.BootstrapForSession(bootstrap, ""); !errors.Is(err, ErrBootstrapUsed) {
		t.Fatalf("replay error = %v", err)
	}
}

func TestBootstrapRejectsWrongTokenWithoutConsumingRealToken(t *testing.T) {
	manager, bootstrap, err := NewManager(bytes.NewReader(bytes.Repeat([]byte{0x21}, 96)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.BootstrapForSession("wrong", ""); !errors.Is(err, ErrInvalidBootstrap) {
		t.Fatalf("wrong-token error = %v", err)
	}
	if _, _, err := manager.BootstrapForSession(bootstrap, ""); err != nil {
		t.Fatalf("valid bootstrap after rejection: %v", err)
	}
}

var errRandom = errors.New("random source failed")

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errRandom }

func TestNewManagerPropagatesRandomFailure(t *testing.T) {
	if _, _, err := NewManager(errReader{}); !errors.Is(err, errRandom) {
		t.Fatalf("NewManager error = %v", err)
	}
}

func TestBootstrapPropagatesSessionRandomFailure(t *testing.T) {
	initial := bytes.NewReader(bytes.Repeat([]byte{0x31}, 32))
	manager, bootstrap, err := NewManager(io.MultiReader(initial, errReader{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.BootstrapForSession(bootstrap, ""); !errors.Is(err, errRandom) {
		t.Fatalf("Bootstrap error = %v", err)
	}
	if ok := acceptsRequest(manager, ""); ok {
		t.Fatal("failed bootstrap created a session")
	}
}

// リロードすると CSRF トークンは失われる。ページの中にあったからだ。Cookie は残る
// のでセッションは残る。だがそのためのトークンを得る手段がないと、バイナリを起動
// し直すまでアプリケーションは死んだままだった。リロードがそこまでの代償を払う
// べきではない。
//
// トークンは返すのではなく発行し直す。マネージャが保持するのはハッシュであって
// トークンではない。それが、メモリの漏洩を全セッションのトークンの漏洩にしない
// 性質であり、発行し直す方式はそれを保つ。
// countingReader はバイト列のパターンを繰り返さないので、そこから引いた二つの
// トークンは、本番でトークンが異なるのと同じ理由で異なる。
type countingReader struct{ next byte }

func (r *countingReader) Read(p []byte) (int, error) {
	for index := range p {
		r.next++
		p[index] = r.next
	}
	return len(p), nil
}

func TestRenewCSRFIssuesAWorkingTokenWithoutDisconnectingAnotherTab(t *testing.T) {
	// 変動する乱数源。定数の乱数源ではすべてのトークンが同一になり、古いトークンを
	// そのまま返す実装でもこのテストが通ってしまう。
	manager, bootstrap, err := NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}

	renewed, ok := manager.RenewCSRF(credentials.SessionID, credentials.CSRFToken)
	if !ok || renewed == "" {
		t.Fatalf("RenewCSRF = %q, %v", renewed, ok)
	}
	if renewed == credentials.CSRFToken {
		t.Error("the renewed token is the old one")
	}
	if !manager.VerifyCSRF(credentials.SessionID, renewed) {
		t.Error("the renewed token does not verify")
	}
	if !manager.VerifyCSRF(credentials.SessionID, credentials.CSRFToken) {
		t.Error("renewal disconnected another tab holding the previous token")
	}
}

// 開いたままのタブは状態を定期的に読むので、その token は使われ続ける。
// ほかのタブの再読み込みや、開いては閉じたタブが何度あっても、そのタブは切れない。
func TestATabThatKeepsUsingItsTokenOutlivesReloadsAndNewTabs(t *testing.T) {
	manager, bootstrap, err := NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	openTab, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	reloadedTab, _, err := manager.JoinOrIssue(openTab.SessionID, "")
	if err != nil {
		t.Fatal(err)
	}

	const visits = 3 * MaxCSRFTokensPerSession
	for visit := range visits {
		renewed, ok := manager.RenewCSRF(reloadedTab.SessionID, reloadedTab.CSRFToken)
		if !ok {
			t.Fatalf("reload %d could not renew its own token", visit)
		}
		reloadedTab.CSRFToken = renewed
		closedTab, _, err := manager.JoinOrIssue(openTab.SessionID, "")
		if err != nil {
			t.Fatal(err)
		}
		if !manager.VerifyCSRF(closedTab.SessionID, closedTab.CSRFToken) {
			t.Fatalf("new tab %d was refused its own token", visit)
		}
		if !manager.VerifyCSRF(openTab.SessionID, openTab.CSRFToken) {
			t.Fatalf("the open tab was disconnected after %d reloads and new tabs", visit+1)
		}
	}
	if !manager.VerifyCSRF(reloadedTab.SessionID, reloadedTab.CSRFToken) {
		t.Fatal("the reloaded tab lost its latest token")
	}
}

// 「タブを複製」は sessionStorage ごと複製するので、2 つのタブが同じ token を
// 持つ。上限まで token が溜まっていても、片方の renew でもう片方は切れない。
func TestADuplicatedTabKeepsWorkingWhenItsTwinRenewsAtTheBound(t *testing.T) {
	manager, bootstrap, err := NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	original, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	for range MaxCSRFTokensPerSession {
		if _, _, err := manager.JoinOrIssue(original.SessionID, ""); err != nil {
			t.Fatal(err)
		}
		// 新しいタブを開くあいだも、元のタブは状態を読み続ける。
		if !manager.VerifyCSRF(original.SessionID, original.CSRFToken) {
			t.Fatal("the original tab was disconnected while other tabs opened")
		}
	}

	duplicateRenewed, ok := manager.RenewCSRF(original.SessionID, original.CSRFToken)
	if !ok {
		t.Fatal("the duplicated tab could not renew the shared token")
	}
	if !manager.VerifyCSRF(original.SessionID, original.CSRFToken) {
		t.Fatal("the duplicated tab's renewal disconnected the original tab")
	}
	if !manager.VerifyCSRF(original.SessionID, duplicateRenewed) {
		t.Fatal("the duplicated tab's new token does not verify")
	}
}

func TestRenewCSRFRefusesASessionThatIsNotThere(t *testing.T) {
	manager, _, err := NewManager(bytes.NewReader(bytes.Repeat([]byte{0x32}, 4096)))
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := manager.RenewCSRF("not a session", "not a token"); ok {
		t.Error("RenewCSRF answered for a session that does not exist")
	}
}

func TestRenewCSRFRequiresAnExistingTokenAndBoundsTabTokens(t *testing.T) {
	manager, bootstrap, err := NewManager(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.RenewCSRF(credentials.SessionID, "cookie-only attacker"); ok {
		t.Fatal("renewal accepted a token that was never issued")
	}

	current := credentials.CSRFToken
	issued := []string{current}
	for range MaxCSRFTokensPerSession {
		current, _ = manager.RenewCSRF(credentials.SessionID, current)
		issued = append(issued, current)
	}
	if manager.VerifyCSRF(credentials.SessionID, issued[0]) {
		t.Fatal("the oldest token survived beyond the per-session bound")
	}
	for _, value := range issued[1:] {
		if !manager.VerifyCSRF(credentials.SessionID, value) {
			t.Fatal("a token inside the bounded tab window was retired")
		}
	}
}

// ブートストラップは初回の使用で消費される。再発行があることで、最初の一つを
// 表示したプロセスが、標準出力がどこにも届かないバックグラウンドエージェントで
// あっても、ブラウザが入れるようになる。
func TestReissueMintsAWayInWithoutDisturbingTheSessions(t *testing.T) {
	manager, first, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	established, _, err := manager.BootstrapForSession(first, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.BootstrapForSession(first, ""); !errors.Is(err, ErrBootstrapUsed) {
		t.Fatalf("the first bootstrap is still spendable: %v", err)
	}

	second, err := manager.Reissue()
	if err != nil {
		t.Fatalf("Reissue = %v", err)
	}
	if second == first {
		t.Error("the reissued bootstrap is the one that was spent")
	}
	if _, _, err := manager.BootstrapForSession(first, ""); err == nil {
		t.Error("the old bootstrap still works after a reissue")
	}
	if _, _, err := manager.BootstrapForSession(second, ""); err != nil {
		t.Fatalf("the reissued bootstrap does not work: %v", err)
	}
	// すでに存在するセッションには手を触れない。これは、セッションを持たないブラウザ
	// のためのアクセス URLであって、持っているものを終わらせる手段ではない。
	if ok := acceptsRequest(manager, established.SessionID); !ok {
		t.Error("an established session was lost")
	}
}

func TestExpiringSessionStopsAuthenticatingAtItsDeadline(t *testing.T) {
	manager, _, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	manager.Now = func() time.Time { return now }

	credentials, err := manager.IssueExpiring(Expiry{Lifetime: 10 * time.Minute})
	if err != nil {
		t.Fatalf("IssueExpiring = %v", err)
	}
	if !acceptsRequest(manager, credentials.SessionID) {
		t.Fatal("new expiring session was not authenticated")
	}
	if !manager.VerifyCSRF(credentials.SessionID, credentials.CSRFToken) {
		t.Fatal("new expiring session rejected its CSRF token")
	}

	now = now.Add(10 * time.Minute)
	if acceptsRequest(manager, credentials.SessionID) {
		t.Fatal("session remained authenticated at its deadline")
	}
	if manager.VerifyCSRF(credentials.SessionID, credentials.CSRFToken) {
		t.Fatal("expired session still accepted its CSRF token")
	}
	if _, ok := manager.RenewCSRF(credentials.SessionID, credentials.CSRFToken); ok {
		t.Fatal("expired session renewed its CSRF token")
	}
}

func TestRevokingSessionWorksOnce(t *testing.T) {
	manager, _, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := manager.IssueExpiring(Expiry{Lifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	if !manager.Revoke(credentials.SessionID) {
		t.Fatal("first revoke did not find the session")
	}
	if manager.Revoke(credentials.SessionID) {
		t.Fatal("second revoke found an already removed session")
	}
	if acceptsRequest(manager, credentials.SessionID) {
		t.Fatal("revoked session remained authenticated")
	}
}

func TestIssueExpiringRejectsNonPositiveLifetime(t *testing.T) {
	manager, _, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expiry := range []Expiry{
		{Lifetime: 0}, {Lifetime: -time.Second}, {Lifetime: time.Minute, IdleTimeout: -time.Second},
	} {
		if _, err := manager.IssueExpiring(expiry); !errors.Is(err, ErrInvalidLifetime) {
			t.Errorf("IssueExpiring(%+v) = %v, want ErrInvalidLifetime", expiry, err)
		}
	}
}

// 使い続けているセッションは、無操作の期限を過ぎても切れない。
func TestASessionInUseOutlivesItsIdleTimeout(t *testing.T) {
	manager, _, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	manager.Now = func() time.Time { return now }
	credentials, err := manager.IssueExpiring(Expiry{Lifetime: time.Hour, IdleTimeout: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	for range 30 {
		now = now.Add(time.Minute)
		if !acceptsRequest(manager, credentials.SessionID) {
			t.Fatalf("a session used every minute ended at %s", now)
		}
	}
	now = now.Add(10 * time.Minute)
	if acceptsRequest(manager, credentials.SessionID) {
		t.Fatal("a session left unused past its idle timeout still authenticated")
	}
}

// 1 本の長いリクエストの処理中は切れず、無操作の期間は処理を終えた時刻から数える。
// ダウンロードの後に送る完了の通知が断られないためである。
func TestARequestInFlightKeepsItsSessionUntilItEnds(t *testing.T) {
	manager, _, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	manager.Now = func() time.Time { return now }
	credentials, err := manager.IssueExpiring(Expiry{Lifetime: time.Hour, IdleTimeout: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	endRequest, ok := manager.BeginRequest(credentials.SessionID)
	if !ok {
		t.Fatal("a fresh session could not begin a request")
	}
	now = now.Add(30 * time.Minute)
	// 期限切れの掃除は、別のセッションの発行でも走る。
	if _, err := manager.IssueExpiring(Expiry{Lifetime: time.Minute}); err != nil {
		t.Fatal(err)
	}
	endRequest()

	now = now.Add(9 * time.Minute)
	if !acceptsRequest(manager, credentials.SessionID) {
		t.Fatal("the session ended while a request was being served")
	}
}

// 無操作の期限を延ばしても、絶対期限は延びない。
func TestASessionInUseStillEndsAtItsLifetime(t *testing.T) {
	manager, _, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	manager.Now = func() time.Time { return now }
	credentials, err := manager.IssueExpiring(Expiry{Lifetime: time.Hour, IdleTimeout: 10 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	endRequest, ok := manager.BeginRequest(credentials.SessionID)
	if !ok {
		t.Fatal("a fresh session could not begin a request")
	}
	defer endRequest()

	now = now.Add(time.Hour)
	if acceptsRequest(manager, credentials.SessionID) {
		t.Fatal("a session in use outlived its lifetime")
	}
}

func TestIssueExpiringPrunesExpiredCommandSessionsWithoutRemovingBrowserSessions(t *testing.T) {
	manager, bootstrap, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	manager.Now = func() time.Time { return now }

	if _, err := manager.IssueExpiring(Expiry{Lifetime: time.Minute}); err != nil {
		t.Fatal(err)
	}
	browser, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.sessions) != 2 {
		t.Fatalf("sessions before deadline = %d, want 2", len(manager.sessions))
	}

	now = now.Add(time.Minute)
	latest, err := manager.IssueExpiring(Expiry{Lifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.sessions) != 2 {
		t.Fatalf("sessions after sweep = %d, want browser plus latest CLI session", len(manager.sessions))
	}
	if !acceptsRequest(manager, browser.SessionID) || !acceptsRequest(manager, latest.SessionID) {
		t.Fatal("sweep removed an active session")
	}
}

func TestCommandSessionDoesNotConsumeBrowserBootstrap(t *testing.T) {
	manager, bootstrap, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	now := testEpoch
	manager.Now = func() time.Time { return now }
	if _, err := manager.IssueExpiring(Expiry{Lifetime: time.Minute}); err != nil {
		t.Fatalf("IssueExpiring = %v", err)
	}

	browser, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatalf("Bootstrap after IssueExpiring = %v", err)
	}
	if !acceptsRequest(manager, browser.SessionID) {
		t.Fatal("browser session was not authenticated")
	}

	// The command session's short deadline is its own; the browser session
	// keeps the browser lifetime.
	manager.Now = func() time.Time { return now.Add(BrowserSessionIdleTimeout - time.Minute) }
	if !acceptsRequest(manager, browser.SessionID) {
		t.Fatal("browser session expired with the command session")
	}
}

func TestBootstrapAndRecoveryJoinAnExistingBrowserSession(t *testing.T) {
	manager, bootstrap, err := NewManager(bytes.NewReader(bytes.Repeat([]byte{0x77}, 256)))
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}
	joined, setCookie, err := manager.JoinOrIssue(first.SessionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if setCookie || joined.SessionID != first.SessionID {
		t.Fatalf("joined = %#v, setCookie=%t", joined, setCookie)
	}
	if !manager.VerifyCSRF(first.SessionID, first.CSRFToken) || !manager.VerifyCSRF(first.SessionID, joined.CSRFToken) {
		t.Fatal("joining a tab revoked another tab token")
	}

	restarted, _, err := NewManager(bytes.NewReader(bytes.Repeat([]byte{0x78}, 256)))
	if err != nil {
		t.Fatal(err)
	}
	recovered, setCookie, err := restarted.JoinOrIssue(first.SessionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if !setCookie || recovered.SessionID == first.SessionID || !acceptsRequest(restarted, recovered.SessionID) {
		t.Fatalf("recovered = %#v, setCookie=%t", recovered, setCookie)
	}
}

func TestABrowserSessionExpiresWhenIdleAndAfterItsLifetime(t *testing.T) {
	manager, bootstrap, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0).UTC()
	manager.Now = func() time.Time { return now }
	browser, _, err := manager.BootstrapForSession(bootstrap, "")
	if err != nil {
		t.Fatal(err)
	}

	// Used every few hours, the session outlives the idle timeout.
	for range 3 {
		now = now.Add(BrowserSessionIdleTimeout - time.Hour)
		if !acceptsRequest(manager, browser.SessionID) {
			t.Fatalf("session used every %v was rejected", BrowserSessionIdleTimeout-time.Hour)
		}
	}
	// Left alone for the idle timeout, it is gone.
	now = now.Add(BrowserSessionIdleTimeout)
	if acceptsRequest(manager, browser.SessionID) {
		t.Fatal("session idle for the timeout still authenticated")
	}

	// A session used constantly still ends at the absolute lifetime.
	now = time.Unix(1_900_000_000, 0).UTC()
	renewed, _, err := manager.JoinOrIssue("", "")
	if err != nil {
		t.Fatal(err)
	}
	for used := time.Duration(0); used < BrowserSessionLifetime; used += time.Hour {
		now = now.Add(time.Hour)
		if used+time.Hour < BrowserSessionLifetime && !acceptsRequest(manager, renewed.SessionID) {
			t.Fatalf("session used hourly was rejected after %v", used+time.Hour)
		}
	}
	if acceptsRequest(manager, renewed.SessionID) {
		t.Fatal("session past its lifetime still authenticated")
	}
}

// 登録から入ったセッションは、その登録が消えたらまとめて失効する。別の登録や
// CLI のセッションには触らない。
func TestRevokingARegistrationEndsOnlyTheSessionsItEntered(t *testing.T) {
	manager, _, err := NewManager(&countingReader{})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := manager.JoinOrIssue("", "registration-1")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := manager.JoinOrIssue("", "registration-1")
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := manager.JoinOrIssue("", "registration-2")
	if err != nil {
		t.Fatal(err)
	}
	command, err := manager.IssueExpiring(Expiry{Lifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	if revoked := manager.RevokeRegistration("registration-1"); revoked != 2 {
		t.Fatalf("revoked = %d, want 2", revoked)
	}
	if acceptsRequest(manager, first.SessionID) || acceptsRequest(manager, second.SessionID) {
		t.Fatal("a session entered through the removed registration survived")
	}
	if !acceptsRequest(manager, other.SessionID) || !acceptsRequest(manager, command.SessionID) {
		t.Fatal("revoking one registration ended an unrelated session")
	}
	if revoked := manager.RevokeRegistration(""); revoked != 0 {
		t.Fatalf("revoking no registration ended %d sessions", revoked)
	}
}

// acceptsRequest は、engine の middleware と同じく BeginRequest でリクエストを 1 本
// 処理したことにし、そのセッションが受け付けたかを返す。
func acceptsRequest(manager *Manager, sessionID string) bool {
	endRequest, ok := manager.BeginRequest(sessionID)
	if ok {
		endRequest()
	}
	return ok
}

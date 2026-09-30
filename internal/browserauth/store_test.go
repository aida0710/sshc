package browserauth_test

import (
	"bytes"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sshc/internal/browserauth"
	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/storage"
)

var epoch = time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)

// testClock は、テストが時間を進められる時計である。
type testClock struct{ now time.Time }

func (clock *testClock) advance(by time.Duration) { clock.now = clock.now.Add(by) }
func (clock *testClock) read() time.Time          { return clock.now }

func newStore(t *testing.T, random []byte) (*browserauth.Store, *testClock) {
	t.Helper()
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{now: epoch}
	return browserauth.NewStore(workspace, bytes.NewReader(random)).WithClock(clock.read), clock
}

func recover(t *testing.T, store *browserauth.Store, token string) (string, bool) {
	t.Helper()
	recovery, err := store.Recover(token)
	if err != nil {
		t.Fatal(err)
	}
	return recovery.Token, recovery.Accepted()
}

func TestRegisterStoresOnlyAHashAndRecoversAfterRestart(t *testing.T) {
	store, _ := newStore(t, bytes.Repeat([]byte{0x51}, 64))
	if err := store.SetPort(55447); err != nil {
		t.Fatal(err)
	}
	token, issued, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	if !issued || len(token) != 43 {
		t.Fatalf("issued=%t token length=%d", issued, len(token))
	}
	contents, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(contents, []byte(token)) {
		t.Fatal("browser token was stored in plaintext")
	}
	if fresh, issued, err := store.Register(token); err != nil || issued || fresh != "" {
		t.Fatalf("existing registration = (%q, %t, %v)", fresh, issued, err)
	}
	if registered, err := store.HasRegistrations(); err != nil || !registered {
		t.Fatalf("HasRegistrations = (%t, %v)", registered, err)
	}
	if port, err := store.Port(); err != nil || port != 55447 {
		t.Fatalf("Port = (%d, %v)", port, err)
	}
	if _, accepted := recover(t, store, token); !accepted {
		t.Fatal("the registered token did not recover a session")
	}
}

// 復旧のたびに token は差し替わる。奪われた token は次の復旧までしか使えない。
func TestRecoveryRotatesTheTokenAndRetiresTheOldOneAfterTheGrace(t *testing.T) {
	store, clock := newStore(t, randomBytes(t, 32*8))
	if err := store.SetPort(55447); err != nil {
		t.Fatal(err)
	}
	first, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	second, accepted := recover(t, store, first)
	if !accepted || second == "" || second == first {
		t.Fatalf("recover(first) = (%q, %t)", second, accepted)
	}
	// 同じブラウザの別 tab は差し替え前の token で来る。猶予のあいだは同じ新 token を渡す。
	if again, accepted := recover(t, store, first); !accepted || again != second {
		t.Fatalf("a second tab with the retired token got (%q, %t), want the same replacement", again, accepted)
	}
	clock.advance(2 * time.Minute)
	// 猶予を過ぎた退役 token の提示は複製とみなし、登録ごと失効する。
	if _, accepted := recover(t, store, first); accepted {
		t.Fatal("a retired token was accepted after the grace period")
	}
	if _, accepted := recover(t, store, second); accepted {
		t.Fatal("the registration survived a replay of its retired token")
	}
	if registered, err := store.HasRegistrations(); err != nil || registered {
		t.Fatalf("HasRegistrations after replay = (%t, %v)", registered, err)
	}
}

func TestRegistrationsExpireWhenUnusedForTheLifetime(t *testing.T) {
	store, clock := newStore(t, randomBytes(t, 32*4))
	token, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(browserauth.RegistrationLifetime - time.Hour)
	rotated, accepted := recover(t, store, token)
	if !accepted {
		t.Fatal("a registration used within its lifetime was refused")
	}
	clock.advance(browserauth.RegistrationLifetime + time.Hour)
	if _, accepted := recover(t, store, rotated); accepted {
		t.Fatal("a registration unused for longer than its lifetime was accepted")
	}
	if registered, err := store.HasRegistrations(); err != nil || registered {
		t.Fatalf("HasRegistrations after expiry = (%t, %v)", registered, err)
	}
}

// 上限に達したら、登録した順ではなく最後に使った順で消す。毎日使うブラウザは、あとから
// 一度だけ登録されたプライベートウィンドウや別のブラウザに押し出されない。
// recoverで入るブラウザも、`sshc open`（bootstrap）で入り直すブラウザも同じである。
func TestAFullStoreDropsTheRegistrationUnusedForTheLongest(t *testing.T) {
	tests := []struct {
		name string
		// use は、毎日使うブラウザが1回入り直し、次に提示するtokenを返す。
		use func(t *testing.T, store *browserauth.Store, token string) string
	}{
		{name: "recover", use: func(t *testing.T, store *browserauth.Store, token string) string {
			rotated, accepted := recover(t, store, token)
			if !accepted {
				t.Fatal("the browser in daily use could not recover")
			}
			return rotated
		}},
		{name: "bootstrap", use: func(t *testing.T, store *browserauth.Store, token string) string {
			if fresh, issued, err := store.Register(token); err != nil || issued || fresh != "" {
				t.Fatalf("Register(valid) = (%q, %t, %v), want the registration kept", fresh, issued, err)
			}
			return token
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, clock := newStore(t, randomBytes(t, 32*(2*browserauth.MaxRegistrations+4)))
			daily, _, err := store.Register("")
			if err != nil {
				t.Fatal(err)
			}
			var others []string
			for range browserauth.MaxRegistrations {
				clock.advance(24 * time.Hour)
				daily = test.use(t, store, daily)
				other, issued, err := store.Register("")
				if err != nil || !issued {
					t.Fatalf("Register(\"\") = (%t, %v)", issued, err)
				}
				others = append(others, other)
			}

			if _, accepted := recover(t, store, daily); !accepted {
				t.Fatal("the browser used every day was dropped for a newer registration")
			}
			if _, accepted := recover(t, store, others[0]); accepted {
				t.Fatal("the registration unused for the longest survived the limit")
			}
			if _, accepted := recover(t, store, others[1]); !accepted {
				t.Fatal("a registration newer than the dropped one was also dropped")
			}
		})
	}
}

func TestChangingPortRevokesRegistrationsButKeepingItPreservesRestartRecovery(t *testing.T) {
	store, _ := newStore(t, randomBytes(t, 32*4))
	if err := store.SetPort(55447); err != nil {
		t.Fatal(err)
	}
	token, issued, err := store.Register("")
	if err != nil || !issued {
		t.Fatalf("Register = (%q, %t, %v)", token, issued, err)
	}
	if err := store.SetPort(55447); err != nil {
		t.Fatal(err)
	}
	rotated, accepted := recover(t, store, token)
	if !accepted {
		t.Fatal("same-port restart revoked a valid browser registration")
	}

	if err := store.SetPort(56447); err != nil {
		t.Fatal(err)
	}
	if _, accepted := recover(t, store, rotated); accepted {
		t.Fatal("registration issued for the previous port remained valid")
	}
	if registered, err := store.HasRegistrations(); err != nil || registered {
		t.Fatalf("HasRegistrations after port change = (%t, %v)", registered, err)
	}
}

func TestInvalidDocumentIsNotSilentlyReplaced(t *testing.T) {
	store, _ := newStore(t, bytes.Repeat([]byte{0x52}, 32))
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"schemaVersion":2,"registrations":[],"unknown":true}`)
	acltest.WritePrivateFile(t, store.Path(), original)
	if _, _, err := store.Register(""); !errors.Is(err, browserauth.ErrInvalidDocument) {
		t.Fatalf("Register = %v, want ErrInvalidDocument", err)
	}
	contents, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(contents, original) {
		t.Fatal("invalid registration document was overwritten")
	}
}

// 旧形式（ハッシュの一覧だけ）は読まず、登録なしとして始める。
func TestAnOlderDocumentStartsWithoutRegistrations(t *testing.T) {
	store, _ := newStore(t, bytes.Repeat([]byte{0x54}, 32))
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatal(err)
	}
	acltest.WritePrivateFile(t, store.Path(), []byte(`{"schemaVersion":1,"port":55447,"hashes":["AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"]}`))
	if registered, err := store.HasRegistrations(); err != nil || registered {
		t.Fatalf("HasRegistrations = (%t, %v), want none", registered, err)
	}
	if _, issued, err := store.Register(""); err != nil || !issued {
		t.Fatalf("Register on an old document = (%t, %v)", issued, err)
	}
}

func randomBytes(t *testing.T, size int) []byte {
	t.Helper()
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatal(err)
	}
	return buffer
}

func TestForgetRemovesTheRegistrationSoItCannotRecoverAgain(t *testing.T) {
	store, _ := newStore(t, bytes.Repeat([]byte{0x61}, 128))
	if err := store.SetPort(55447); err != nil {
		t.Fatal(err)
	}
	token, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	rotated, accepted := recover(t, store, token)
	if !accepted {
		t.Fatal("registered token was not accepted")
	}

	// The retired token still names the same registration during the grace.
	forgotten, err := store.Forget(token)
	if err != nil || forgotten == "" {
		t.Fatalf("Forget(retired) = (%q, %v), want the registration", forgotten, err)
	}
	if _, accepted := recover(t, store, rotated); accepted {
		t.Fatal("a forgotten registration recovered a session")
	}
	if registered, err := store.HasRegistrations(); err != nil || registered {
		t.Fatalf("HasRegistrations after Forget = (%t, %v)", registered, err)
	}
	if forgotten, err := store.Forget("unknown-token-of-the-right-length-43-chars"); err != nil || forgotten != "" {
		t.Fatalf("Forget(unknown) = (%q, %v), want nothing forgotten", forgotten, err)
	}
}

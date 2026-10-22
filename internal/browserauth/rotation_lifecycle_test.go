package browserauth_test

import (
	"testing"
	"time"

	"sshc/internal/browserauth"
)

func TestSignOutRevokesEveryGraceToken(t *testing.T) {
	store, _ := newStore(t, randomBytes(t, 32*8))
	first, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := recover(t, store, first)
	third, _ := recover(t, store, second)
	if removed, err := store.Forget(third); err != nil || removed == "" {
		t.Fatalf("Forget: %v, %v", removed, err)
	}
	if registered, err := store.HasRegistrations(); err != nil || registered {
		t.Fatalf("registrations remain: %v, %v", registered, err)
	}
	if _, accepted := recover(t, store, first); accepted {
		t.Fatal("signed-out registration still issues a session through its oldest grace token")
	}
}

func TestSigningOutWithAnOlderGraceTokenRevokesTheCurrentRegistration(t *testing.T) {
	store, _ := newStore(t, randomBytes(t, 32*8))
	first, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := recover(t, store, first)
	third, _ := recover(t, store, second)
	if removed, err := store.Forget(first); err != nil || removed == "" {
		t.Fatalf("sign-out using grace token: %v, %v", removed, err)
	}
	for _, token := range []string{first, second, third} {
		if _, accepted := recover(t, store, token); accepted {
			t.Fatal("a signed-out token family can still recover")
		}
	}
}

func TestNewRotationsDoNotExtendAnOlderTokensGrace(t *testing.T) {
	store, clock := newStore(t, randomBytes(t, 32*8))
	first, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := recover(t, store, first)
	clock.advance(40 * time.Second)
	third, _ := recover(t, store, second)
	clock.advance(21 * time.Second)
	late, err := store.Recover(first)
	if err != nil {
		t.Fatal(err)
	}
	if late.Accepted() {
		t.Fatal("later rotation extended the oldest token's grace")
	}
	// 猶予の切れた退役 token は、何世代前でも複製として登録ごと消す。
	if late.Stolen == "" {
		t.Fatal("the oldest token after its grace was not detected as a clone")
	}
	if _, accepted := recover(t, store, third); accepted {
		t.Fatal("the current token survived the detected clone")
	}
}

func TestGraceReturnsTheLatestToken(t *testing.T) {
	store, _ := newStore(t, randomBytes(t, 32*8))
	first, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := recover(t, store, first)
	third, _ := recover(t, store, second)
	latest, accepted := recover(t, store, first)
	if !accepted || latest != third {
		t.Fatal("a delayed tab receives a retired token and overwrites shared localStorage")
	}
}

// 登録の識別子は token を差し替えても変わらない。複製を検知して消した登録も
// 同じ識別子で知らせるので、その登録から入ったセッションをまとめて失効できる。
func TestARegistrationKeepsItsIdentifierAcrossRotationsAndTheft(t *testing.T) {
	store, clock := newStore(t, randomBytes(t, 32*8))
	first, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := store.Recover(first)
	if err != nil || !rotated.Accepted() {
		t.Fatalf("Recover(first) = %+v, %v", rotated, err)
	}
	again, err := store.Recover(rotated.Token)
	if err != nil || !again.Accepted() {
		t.Fatalf("Recover(rotated) = %+v, %v", again, err)
	}
	if rotated.Registration == "" || again.Registration != rotated.Registration {
		t.Fatalf("identifiers across a rotation = %q, %q", rotated.Registration, again.Registration)
	}

	clock.advance(2 * time.Minute)
	copied, err := store.Recover(rotated.Token)
	if err != nil || copied.Accepted() {
		t.Fatalf("Recover(retired after the grace) = %+v, %v", copied, err)
	}
	if copied.Stolen != rotated.Registration {
		t.Fatalf("stolen = %q, want %q", copied.Stolen, rotated.Registration)
	}
}

// 盗んだ側が続けて何度 recover しても、正規のブラウザが猶予の後に元の token を
// 出せば複製として検知し、盗んだ側の現在の token も使えなくなる。
func TestAnOriginalTokenIsDetectedAsACloneAfterSeveralRotations(t *testing.T) {
	store, clock := newStore(t, randomBytes(t, 32*8))
	original, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	stolen1, ok := recover(t, store, original)
	if !ok {
		t.Fatal("first recover with the stolen token was refused")
	}
	stolen2, ok := recover(t, store, stolen1)
	if !ok {
		t.Fatal("second recover with the stolen token was refused")
	}
	clock.advance(2 * time.Minute)
	genuine, err := store.Recover(original)
	if err != nil {
		t.Fatal(err)
	}
	if genuine.Accepted() || genuine.Stolen == "" {
		t.Fatalf("Recover(original after two rotations) = %+v; want the clone detected", genuine)
	}
	if _, accepted := recover(t, store, stolen2); accepted {
		t.Fatal("the thief's current token still recovers after the clone was detected")
	}
}

// 覚えておく退役 token は MaxRetiredTokens 世代まで。それより古い token は、
// 複製として検知できず、ただ断られる。
func TestATokenOlderThanTheRetiredGenerationsIsOnlyRefused(t *testing.T) {
	store, clock := newStore(t, randomBytes(t, 32*(browserauth.MaxRetiredTokens+4)))
	original, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	oldest := original
	current := original
	for range browserauth.MaxRetiredTokens + 1 {
		next, ok := recover(t, store, current)
		if !ok {
			t.Fatal("rotation was refused")
		}
		current = next
	}
	clock.advance(2 * time.Minute)
	forgotten, err := store.Recover(oldest)
	if err != nil || forgotten.Accepted() || forgotten.Stolen != "" {
		t.Fatalf("Recover(beyond the retired generations) = %+v, %v; want a plain refusal", forgotten, err)
	}
	if _, accepted := recover(t, store, current); !accepted {
		t.Fatal("the current token stopped working")
	}
}

package browserauth_test

import (
	"testing"
	"time"
)

func TestSignOutRevokesEveryGraceToken(t *testing.T) {
	store, _ := newStore(t, randomBytes(t, 32*8))
	first, _, err := store.Register("")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := recover(t, store, first)
	third, _ := recover(t, store, second)
	if removed, err := store.Forget(third); err != nil || !removed {
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
	if removed, err := store.Forget(first); err != nil || !removed {
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
	if _, accepted := recover(t, store, first); accepted {
		t.Fatal("later rotation extended the oldest token's grace")
	}
	if _, accepted := recover(t, store, third); !accepted {
		t.Fatal("current token stopped working with the oldest token's expiry")
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

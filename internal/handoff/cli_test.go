package handoff

import (
	"errors"
	"net/http"
	"testing"
)

func TestAStatusAnswerThatLacksAFieldOrContradictsItselfIsRefused(t *testing.T) {
	yes, no, two, negative := true, false, 2, -1
	if status, err := (EngineStatusWire{Vault: &yes, Unlocked: &no, Sessions: &two, Version: "v1"}).Status(); err != nil || status.Sessions != 2 || status.Unlocked {
		t.Fatalf("Status() = %+v, %v", status, err)
	}
	for name, wire := range map[string]EngineStatusWire{
		"no vault field":            {Unlocked: &no, Sessions: &two},
		"no session count":          {Vault: &yes, Unlocked: &yes},
		"negative count":            {Vault: &yes, Unlocked: &yes, Sessions: &negative},
		"open vault that is absent": {Vault: &no, Unlocked: &yes, Sessions: &two},
	} {
		if _, err := wire.Status(); !errors.Is(err, ErrInvalidCLIAnswer) {
			t.Errorf("%s: Status() = %v, want ErrInvalidCLIAnswer", name, err)
		}
	}
}

func TestASessionAnswerNeedsBothTheCookieAndTheToken(t *testing.T) {
	cookie := []*http.Cookie{{Name: SessionCookie, Value: "session"}}
	if credentials, err := ReadSessionAnswer([]byte(`{"csrfToken":"token","browserToken":"later field"}`), cookie); err != nil || credentials != (SessionCredentials{Cookie: "session", CSRF: "token"}) {
		t.Fatalf("ReadSessionAnswer = %+v, %v", credentials, err)
	}
	if _, err := ReadSessionAnswer([]byte(`{"csrfToken":"token"}`), nil); !errors.Is(err, ErrInvalidCLIAnswer) {
		t.Errorf("without the cookie = %v", err)
	}
	if credentials, err := ReadSessionAnswer([]byte(`{}`), cookie); !errors.Is(err, ErrInvalidCLIAnswer) || credentials.Cookie != "session" {
		t.Errorf("without the token = %+v, %v; want the cookie kept so it can be returned", credentials, err)
	}
}

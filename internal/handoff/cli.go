package handoff

import (
	"encoding/json"
	"errors"
	"net/http"
)

// The engine's /cli/ steps, as the sshc command and a run target's engine both call them: prove the
// engine owns the handoff (ChallengePath), ask its state (StatusPath), and borrow an API session
// (SessionPath) that later requests carry as SessionCookie and CSRFHeader. The engine serves these
// names (internal/httpserver), so every side spells them from here.
const (
	ChallengePath = "/cli/challenge"
	StatusPath    = "/cli/status"
	SessionPath   = "/cli/session"
	SessionCookie = "sshc_session"
	CSRFHeader    = "X-SSHC-CSRF"
	// ActionHeader は、一度限りの確認のトークンを運ぶヘッダーである。実行先のエンジンへ確認の
	// 要る操作を送るときに、このマシンのエンジンも使う。
	ActionHeader = "X-SSHC-Action"
)

// ErrInvalidCLIAnswer is an answer to a /cli/ step that lacks what the engine always sends.
var ErrInvalidCLIAnswer = errors.New("the engine's answer is incomplete or inconsistent")

// EngineStatus is the engine's answer at StatusPath.
type EngineStatus struct {
	Owner           Owner
	Version         string
	ProtocolVersion int
	// Vault says whether a vault exists; Unlocked whether it is open, which it cannot be without one.
	Vault        bool
	Unlocked     bool
	Passwordless bool
	// Sessions counts the terminals still running.
	Sessions int
}

// EngineStatusWire is the StatusPath answer as sent. The required fields are pointers, so a missing one
// is told from false or zero. Each caller decodes it its own way: the sshc command strictly, since it only
// talks to an engine of its own version; a run target's engine leniently, since that engine may be a
// newer sshc that sends more.
type EngineStatusWire struct {
	Owner           Owner  `json:"owner"`
	Version         string `json:"version"`
	ProtocolVersion int    `json:"protocolVersion"`
	Vault           *bool  `json:"vault"`
	Unlocked        *bool  `json:"unlocked"`
	Passwordless    bool   `json:"passwordless"`
	Sessions        *int   `json:"sessions"`
}

// Status checks the answer and returns it. It refuses one that lacks a required field or contradicts
// itself (an open vault that does not exist, a negative count).
func (wire EngineStatusWire) Status() (EngineStatus, error) {
	if wire.Vault == nil || wire.Unlocked == nil || wire.Sessions == nil || *wire.Sessions < 0 || (!*wire.Vault && *wire.Unlocked) {
		return EngineStatus{}, ErrInvalidCLIAnswer
	}
	return EngineStatus{
		Owner: wire.Owner, Version: wire.Version, ProtocolVersion: wire.ProtocolVersion,
		Vault: *wire.Vault, Unlocked: *wire.Unlocked, Passwordless: wire.Passwordless, Sessions: *wire.Sessions,
	}, nil
}

// SessionCredentials are what a SessionPath answer lends: the session cookie and the CSRF token that go
// with every later API request.
type SessionCredentials struct {
	Cookie string
	CSRF   string
}

// ReadSessionAnswer reads a successful SessionPath answer from its body and cookies. The caller reads the
// body (and may clear it afterwards: it holds the CSRF token). Fields it does not know are skipped, since
// a run target may be a newer sshc.
func ReadSessionAnswer(body []byte, cookies []*http.Cookie) (SessionCredentials, error) {
	var answer struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return SessionCredentials{}, errors.Join(ErrInvalidCLIAnswer, err)
	}
	credentials := SessionCredentials{CSRF: answer.CSRFToken}
	for _, cookie := range cookies {
		if cookie.Name == SessionCookie && cookie.Value != "" {
			credentials.Cookie = cookie.Value
			break
		}
	}
	if credentials.Cookie == "" || credentials.CSRF == "" {
		return credentials, ErrInvalidCLIAnswer
	}
	return credentials, nil
}

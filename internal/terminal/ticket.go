package terminal

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"io"
	"sync"
	"time"
)

// TicketTTL は、発行から引き換えまでに許される時間である。
const TicketTTL = 10 * time.Second

// Tickets は、WebSocket のアップグレードを認可する使い捨ての秘密を持つ。
type Tickets struct {
	// Now と Random は、テストが時計と値を固定するためにここにある。
	Now    func() time.Time
	Random io.Reader
	// TTL は 0 なら TicketTTL。
	TTL time.Duration

	mutex  sync.Mutex
	issued map[string]ticket
}

type ticket struct {
	session string
	cursor  uint64
	expires time.Time
}

// TicketClaim is the terminal stream position bound to one redeemed ticket.
// Cursor is kept inside the single-use secret instead of on the WebSocket URL,
// so a caller cannot change which bytes are replayed after the ticket is issued.
type TicketClaim struct {
	SessionID string
	Cursor    uint64
}

func (t *Tickets) now() time.Time {
	if t.Now == nil {
		return time.Now()
	}
	return t.Now()
}

func (t *Tickets) ttl() time.Duration {
	if t.TTL <= 0 {
		return TicketTTL
	}
	return t.TTL
}

func (t *Tickets) random() io.Reader {
	if t.Random == nil {
		return rand.Reader
	}
	return t.Random
}

// Issue は、ひとつのセッションと再生開始位置に束縛された使い捨てのチケットを作る。
func (t *Tickets) Issue(sessionID string, cursor uint64) (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(t.random(), raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.issued == nil {
		t.issued = map[string]ticket{}
	}
	t.sweep()
	t.issued[token] = ticket{session: sessionID, cursor: cursor, expires: t.now().Add(t.ttl())}
	return token, nil
}

// Redeem は、そのチケットが束縛しているセッションと再生開始位置を返し、
// チケットを使い切る。
func (t *Tickets) Redeem(token string) (TicketClaim, bool) {
	if token == "" {
		return TicketClaim{}, false
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.sweep()
	for candidate, entry := range t.issued {
		if len(candidate) == len(token) &&
			subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) == 1 {
			delete(t.issued, candidate)
			return TicketClaim{SessionID: entry.session, Cursor: entry.cursor}, true
		}
	}
	return TicketClaim{}, false
}

// Forget は、あるセッションに対して発行済みのチケットをすべて捨てる。
func (t *Tickets) Forget(sessionID string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	for token, entry := range t.issued {
		if entry.session == sessionID {
			delete(t.issued, token)
		}
	}
}

func (t *Tickets) sweep() {
	now := t.now()
	for token, entry := range t.issued {
		if !entry.expires.After(now) {
			delete(t.issued, token)
		}
	}
}

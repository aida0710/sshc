package httpserver

import (
	"crypto/rand"
	"crypto/subtle"
	"sync"

	"sshc/internal/randomid"
)

// maxMediaTicketsPerSession bounds what one browser session can keep open.
// A preview holds one ticket while it plays; a few more cover previews that
// closed without revoking theirs.
const maxMediaTicketsPerSession = 16

// mediaTickets authorize <audio> and <video> to read one file. The elements
// cannot send the CSRF header the API asks for on every read, so a ticket,
// issued through a request that did send it, stands in for the header on the
// URL. Unlike a terminal ticket, it is read many times: the player asks for
// the ranges it needs for as long as it plays. It is bound to the browser
// session that asked for it, and to one alias and path, and lives until the
// preview revokes it or the session issues too many.
type mediaTickets struct {
	mutex  sync.Mutex
	issued []mediaTicket
}

type mediaTicket struct {
	token   string
	session string
	alias   string
	path    string
}

type mediaTicketClaim struct {
	alias string
	path  string
}

func (t *mediaTickets) issue(session, alias, path string) (string, error) {
	token, err := randomid.Token(rand.Reader)
	if err != nil {
		return "", err
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()
	held := 0
	for _, ticket := range t.issued {
		if ticket.session == session {
			held++
		}
	}
	if held >= maxMediaTicketsPerSession {
		t.dropOldestOf(session)
	}
	t.issued = append(t.issued, mediaTicket{token: token, session: session, alias: alias, path: path})
	return token, nil
}

func (t *mediaTickets) dropOldestOf(session string) {
	for index, ticket := range t.issued {
		if ticket.session == session {
			t.issued = append(t.issued[:index], t.issued[index+1:]...)
			return
		}
	}
}

// claim returns the file a ticket names, when the same browser session holds it.
func (t *mediaTickets) claim(session, token string) (mediaTicketClaim, bool) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	index := t.find(session, token)
	if index < 0 {
		return mediaTicketClaim{}, false
	}
	return mediaTicketClaim{alias: t.issued[index].alias, path: t.issued[index].path}, true
}

func (t *mediaTickets) revoke(session, token string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if index := t.find(session, token); index >= 0 {
		t.issued = append(t.issued[:index], t.issued[index+1:]...)
	}
}

func (t *mediaTickets) find(session, token string) int {
	if token == "" {
		return -1
	}
	for index, ticket := range t.issued {
		if ticket.session == session && len(ticket.token) == len(token) &&
			subtle.ConstantTimeCompare([]byte(ticket.token), []byte(token)) == 1 {
			return index
		}
	}
	return -1
}

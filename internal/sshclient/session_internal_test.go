package sshclient

import (
	"io"
	"testing"

	"sshc/internal/terminal"
)

type observedCloser struct{ closed chan struct{} }

func (c observedCloser) Close() error {
	close(c.closed)
	return nil
}

func TestAClosedSessionRejectsLateAttachmentAndClosesItsTransports(t *testing.T) {
	session := newSession(terminal.Size{Cols: 80, Rows: 24}, nil)
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	_, attached := session.attach(nil, []io.Closer{observedCloser{closed: closed}})
	if attached {
		t.Fatal("a closed Session accepted a late transport")
	}
	select {
	case <-closed:
	default:
		t.Fatal("the transport rejected by Session.attach leaked")
	}
}

func TestSessionCloseReleasesATransportAttachedBeforeItsChannel(t *testing.T) {
	session := newSession(terminal.Size{Cols: 80, Rows: 24}, nil)
	closed := make(chan struct{})
	if _, attached := session.attach(nil, []io.Closer{observedCloser{closed: closed}}); !attached {
		t.Fatal("an open Session rejected its transport")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("Session.Close leaked the transport it owned before NewSession completed")
	}
}

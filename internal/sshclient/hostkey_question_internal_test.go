package sshclient

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/terminal"
)

func offeredKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// askingSession is a connecting session whose first hop offered key and now
// asks, in the terminal, whether to trust it. The answer arrives on the channel.
func askingSession(t *testing.T, key ssh.PublicKey) (*Session, <-chan bool) {
	t.Helper()
	session := newSession(terminal.Size{Cols: 80, Rows: 24}, nil)
	t.Cleanup(func() { _ = session.Close() })
	go func() { _, _ = io.Copy(io.Discard, session.reader) }()
	target := Target{Alias: "web-01", HostName: "10.0.2.15"}
	session.setProgress(terminal.ConnectionProgress{
		Phase: terminal.ConnectionHostKey, Alias: target.Alias, HostName: target.HostName, Hop: 1, Hops: 1,
		HostKey: &terminal.OfferedHostKey{Type: key.Type(), Fingerprint: ssh.FingerprintSHA256(key)},
	})
	answered := make(chan bool, 1)
	go func() {
		accepted, _ := session.Prompter().Confirm(UnknownHostPrompt(target, key))
		answered <- accepted
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, asking := session.HostKeyQuestion(); asking {
			return session, answered
		}
		if time.Now().After(deadline) {
			t.Fatal("the session never told the question it asks")
		}
		time.Sleep(time.Millisecond)
	}
}

func answerOf(t *testing.T, answered <-chan bool) bool {
	t.Helper()
	select {
	case accepted := <-answered:
		return accepted
	case <-time.After(5 * time.Second):
		t.Fatal("the question was never answered")
		return false
	}
}

func TestASessionTellsTheHostKeyItAsksAbout(t *testing.T) {
	key := offeredKey(t)
	session, _ := askingSession(t, key)
	question, _ := session.HostKeyQuestion()
	want := terminal.HostKeyQuestion{Alias: "web-01", HostName: "10.0.2.15", KeyType: key.Type(), Fingerprint: ssh.FingerprintSHA256(key)}
	if question != want {
		t.Fatalf("question = %+v, want %+v", question, want)
	}
}

func TestAButtonAnswersTheHostKeyQuestionOverWhatWasTypedIntoIt(t *testing.T) {
	key := offeredKey(t)
	session, answered := askingSession(t, key)
	if _, err := session.input.Write([]byte("nope")); err != nil {
		t.Fatal(err)
	}
	if !session.AnswerHostKey(ssh.FingerprintSHA256(key), true) {
		t.Fatal("the open question did not take the answer")
	}
	if !answerOf(t, answered) {
		t.Fatal("the key was not trusted")
	}
	if _, asking := session.HostKeyQuestion(); asking {
		t.Fatal("the answered question is still told")
	}
}

func TestAnAnswerAboutAnotherKeyIsNotTaken(t *testing.T) {
	// With ProxyJump, a stale button for the first hop must not answer the second hop's key.
	key := offeredKey(t)
	session, answered := askingSession(t, key)
	if session.AnswerHostKey(ssh.FingerprintSHA256(offeredKey(t)), true) {
		t.Fatal("an answer about another key was taken")
	}
	if _, err := session.input.Write([]byte("no\r")); err != nil {
		t.Fatal(err)
	}
	if answerOf(t, answered) {
		t.Fatal("the key was trusted without being asked about")
	}
}

func TestAnAnswerAfterTheQuestionClosedIsNotWrittenAnywhere(t *testing.T) {
	key := offeredKey(t)
	session, answered := askingSession(t, key)
	if _, err := session.input.Write([]byte("yes\r")); err != nil {
		t.Fatal(err)
	}
	answerOf(t, answered)
	// The connection went on and is usable: a late yes would run as a command in the shell.
	session.input.markUsable()
	if session.AnswerHostKey(ssh.FingerprintSHA256(key), true) {
		t.Fatal("a late answer was taken")
	}
	session.input.mutex.Lock()
	defer session.input.mutex.Unlock()
	if len(session.input.data) != 0 {
		t.Fatalf("a late answer was written: %q", session.input.data)
	}
}

func TestAPasswordPromptIsNeverAnsweredByTheHostKeyButtons(t *testing.T) {
	session := newSession(terminal.Size{Cols: 80, Rows: 24}, nil)
	t.Cleanup(func() { _ = session.Close() })
	go func() { _, _ = io.Copy(io.Discard, session.reader) }()
	go func() { _, _ = session.Prompter().Secret("deploy@web-01's password: ") }()
	deadline := time.Now().Add(5 * time.Second)
	for !session.AwaitingPrompt() {
		if time.Now().After(deadline) {
			t.Fatal("the password prompt never opened")
		}
		time.Sleep(time.Millisecond)
	}
	if session.AnswerHostKey("SHA256:", true) {
		t.Fatal("a host key answer went into a password prompt")
	}
	if _, asking := session.HostKeyQuestion(); asking {
		t.Fatal("a password prompt was told as a host key question")
	}
}

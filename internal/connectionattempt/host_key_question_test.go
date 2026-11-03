package connectionattempt

import (
	"context"
	"errors"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

const (
	askedFingerprint = "SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E"
	// askedQuestion は、sshclient.UnknownHostPrompt が同じ鍵について出す問いの文である。
	askedQuestion = "The authenticity of host 'web' cannot be established.\r\n" +
		"SSH-ED25519 key fingerprint is " + askedFingerprint + ".\r\n" +
		"Are you sure you want to continue connecting (yes/no)? "
)

// checkingKey は、接続が askedFingerprint の鍵を確かめている段階である。
var checkingKey = terminal.ConnectionProgress{
	Phase: terminal.ConnectionHostKey, Alias: "web", HostName: "192.0.2.10", User: "ops", Hop: 1, Hops: 1,
	HostKey: &terminal.OfferedHostKey{Type: "ssh-ed25519", Fingerprint: askedFingerprint},
}

type confirmation struct {
	accept bool
	err    error
}

// ask は、attempt の接続が問いを出したときと同じく、別の goroutine で答えを待つ。
func ask(ctx context.Context, attempt *Attempt) <-chan confirmation {
	answered := make(chan confirmation, 1)
	go func() {
		accept, err := attempt.ConfirmHostKey(ctx, askedQuestion)
		answered <- confirmation{accept: accept, err: err}
	}()
	return answered
}

func waitForQuestion(t *testing.T, registry *Registry) {
	t.Helper()
	if !testwait.Reached(func() bool {
		snapshot, _ := registry.Snapshot("web", validID)
		return snapshot.HostKeyQuestion != nil
	}) {
		t.Fatal("the question never showed in the attempt")
	}
}

func TestAnAttemptShowsTheHostKeyQuestionAndHandsTheAnswerToTheConnection(t *testing.T) {
	registry := NewRegistry()
	attempt, _ := registry.Begin("web", validID)
	attempt.Progress(checkingKey)
	answered := ask(context.Background(), attempt)
	waitForQuestion(t, registry)

	snapshot, _ := registry.Snapshot("web", validID)
	want := terminal.HostKeyQuestion{Alias: "web", HostName: "192.0.2.10", KeyType: "ssh-ed25519", Fingerprint: askedFingerprint}
	if *snapshot.HostKeyQuestion != want {
		t.Errorf("question = %+v, want %+v", *snapshot.HostKeyQuestion, want)
	}
	if err := registry.AnswerHostKey("web", validID, askedFingerprint, true); err != nil {
		t.Fatalf("AnswerHostKey = %v", err)
	}
	if got := <-answered; !got.accept || got.err != nil {
		t.Errorf("the connection got %+v, want accept", got)
	}
	if snapshot, _ := registry.Snapshot("web", validID); snapshot.HostKeyQuestion != nil {
		t.Errorf("an answered question is still shown: %+v", snapshot.HostKeyQuestion)
	}
}

// 画面が古い問い（別の鍵）を見て押した答えや、2度目の答えは、どの問いにも入らない。
func TestAnAnswerAboutAnotherKeyOrASecondAnswerChangesNothing(t *testing.T) {
	registry := NewRegistry()
	attempt, _ := registry.Begin("web", validID)
	attempt.Progress(checkingKey)
	answered := ask(context.Background(), attempt)
	waitForQuestion(t, registry)

	if err := registry.AnswerHostKey("web", validID, "SHA256:another", true); !errors.Is(err, ErrNoHostKeyQuestion) {
		t.Errorf("an answer about another key = %v, want ErrNoHostKeyQuestion", err)
	}
	if err := registry.AnswerHostKey("web", validID, askedFingerprint, false); err != nil {
		t.Fatalf("AnswerHostKey = %v", err)
	}
	if err := registry.AnswerHostKey("web", validID, askedFingerprint, true); !errors.Is(err, ErrNoHostKeyQuestion) {
		t.Errorf("a second answer = %v, want ErrNoHostKeyQuestion", err)
	}
	if got := <-answered; got.accept || got.err != nil {
		t.Errorf("the connection got %+v, want the first answer (decline)", got)
	}
}

// 段階が別の鍵（ProxyJump の次の段）へ進んでいれば、前の問いは示さず、答えも受け取らない。
func TestAQuestionIsShownOnlyWhileTheConnectionChecksThatKey(t *testing.T) {
	registry := NewRegistry()
	attempt, _ := registry.Begin("web", validID)
	attempt.Progress(terminal.ConnectionProgress{Phase: terminal.ConnectionAuthenticating, Alias: "web", Hop: 1, Hops: 1})
	answered := ask(context.Background(), attempt)
	if !testwait.Reached(func() bool {
		registry.mutex.Lock()
		defer registry.mutex.Unlock()
		return attempt.hostKeyQuestion != nil
	}) {
		t.Fatal("the connection never began to wait")
	}
	if snapshot, _ := registry.Snapshot("web", validID); snapshot.HostKeyQuestion != nil {
		t.Errorf("a question was shown for a stage that checks no key: %+v", snapshot.HostKeyQuestion)
	}
	if err := registry.AnswerHostKey("web", validID, askedFingerprint, true); !errors.Is(err, ErrNoHostKeyQuestion) {
		t.Errorf("AnswerHostKey = %v, want ErrNoHostKeyQuestion", err)
	}
	attempt.Progress(checkingKey)
	if err := registry.AnswerHostKey("web", validID, askedFingerprint, true); err != nil {
		t.Fatalf("AnswerHostKey once the key is checked = %v", err)
	}
	<-answered
}

// 尋ねた要求が終わったら（タブが別の接続先へ移った、画面を閉じた）、答えを待たずに返り、問いを消す。
func TestAQuestionNobodyCanAnswerAnyMoreEndsWithTheRequest(t *testing.T) {
	registry := NewRegistry()
	attempt, _ := registry.Begin("web", validID)
	attempt.Progress(checkingKey)
	ctx, cancel := context.WithCancel(context.Background())
	answered := ask(ctx, attempt)
	waitForQuestion(t, registry)

	cancel()
	if got := <-answered; got.accept || !errors.Is(got.err, context.Canceled) {
		t.Errorf("the connection got %+v, want context.Canceled", got)
	}
	if snapshot, _ := registry.Snapshot("web", validID); snapshot.HostKeyQuestion != nil {
		t.Errorf("the question outlived its request: %+v", snapshot.HostKeyQuestion)
	}
	if err := registry.AnswerHostKey("web", validID, askedFingerprint, true); !errors.Is(err, ErrNoHostKeyQuestion) {
		t.Errorf("AnswerHostKey = %v, want ErrNoHostKeyQuestion", err)
	}
}

func TestAnAnswerToAnAttemptNotKeptIsNotFound(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Begin("web", validID); err != nil {
		t.Fatal(err)
	}
	if err := registry.AnswerHostKey("db", validID, askedFingerprint, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("another alias = %v, want ErrNotFound", err)
	}
}

package terminal_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

// recordedLog is a session log that keeps what it was handed, in order.
type recordedLog struct {
	mutex   sync.Mutex
	written strings.Builder
	notes   []string
	secrets []string
	sizes   []terminal.Size
	closed  bool
}

func (l *recordedLog) Write(output []byte) { l.mutex.Lock(); l.written.Write(output); l.mutex.Unlock() }
func (l *recordedLog) Resize(size terminal.Size) {
	l.mutex.Lock()
	l.sizes = append(l.sizes, size)
	l.mutex.Unlock()
}
func (l *recordedLog) Note(text string) {
	l.mutex.Lock()
	l.notes = append(l.notes, text)
	l.mutex.Unlock()
}
func (l *recordedLog) Hide(secrets []string) {
	l.mutex.Lock()
	l.secrets = append(l.secrets, secrets...)
	l.mutex.Unlock()
}
func (l *recordedLog) Close() { l.mutex.Lock(); l.closed = true; l.mutex.Unlock() }

func (l *recordedLog) output() string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.written.String()
}

func (l *recordedLog) noted() []string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return append([]string(nil), l.notes...)
}

func (l *recordedLog) isClosed() bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.closed
}

func openLogged(t *testing.T, spec terminal.Spec) (*terminal.Session, *recordedLog) {
	t.Helper()
	log := &recordedLog{}
	registry, _ := newFastRegistry()
	registry.OpenLog = func(terminal.LogTarget) (terminal.OutputLog, error) { return log, nil }
	session, err := registry.Open(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	return session, log
}

// Only the shell's own output reaches the log of an SSH session: not the authentication prompt and what was typed
// at it, written before the connection was ready, nor sshc's own lines.
func TestTheLogOfAnSSHSessionKeepsOnlyTheShellsOutputOnceItIsReady(t *testing.T) {
	spy := &shellOpenSpy{}
	session, log := openLogged(t, terminal.Spec{Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open})
	shell := spy.at(0)
	shell.feed("Password for aida@web: ")
	shell.feed("********\r\n")
	shell.finishOpen(nil)
	testwait.Until(t, shell.watching)
	shell.shell("aida@web:~$ ")
	shell.feed("[sshc] a line of sshc's own\r\n")

	testwait.Until(t, func() bool { return strings.Contains(log.output(), "aida@web:~$ ") })
	if got := log.output(); strings.Contains(got, "Password") || strings.Contains(got, "[sshc]") {
		t.Fatalf("log = %q, want the shell's output only", got)
	}
	if got := log.noted(); len(got) != 1 || got[0] != "接続しました。" {
		t.Fatalf("notes = %q", got)
	}
	shell.exit(terminal.ExitInfo{Code: 0})
	testwait.Until(t, func() bool { return !session.Live() && log.isClosed() })
	if got := log.noted(); strings.Join(got, "|") != "接続しました。|接続が切れました。|セッションが終わりました。" {
		t.Fatalf("notes = %q", got)
	}
}

func TestTheLogSaysWhenTheConnectionWasLostAndWhenItCameBack(t *testing.T) {
	spy := &shellOpenSpy{}
	_, log := openLogged(t, terminal.Spec{Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open})
	first := spy.at(0)
	first.finishOpen(nil)
	testwait.Until(t, first.watching)
	first.exit(transportLost)
	testwait.Until(t, func() bool { return spy.at(1) != nil })
	second := spy.at(1)
	second.finishOpen(nil)
	testwait.Until(t, second.watching)
	second.shell("back$ ")
	testwait.Until(t, func() bool { return strings.Contains(log.output(), "back$ ") })
	if got := log.noted(); strings.Join(got, "|") != "接続しました。|接続が切れました。|再接続しました。" {
		t.Fatalf("notes = %q", got)
	}
	if first.watching() {
		t.Fatal("the log still watches the shell that was lost")
	}
	second.exit(terminal.ExitInfo{Code: 0})
}

func TestTheLogOfALocalShellKeepsWhatItPrints(t *testing.T) {
	process := newFakeProcess()
	session, log := openLogged(t, terminal.Spec{
		Kind: terminal.KindShell, Title: "bash",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	})
	process.feed("$ ls\r\nnotes.txt\r\n")
	testwait.Until(t, func() bool { return log.output() == "$ ls\r\nnotes.txt\r\n" })
	if err := session.Resize(terminal.Size{Cols: 100, Rows: 30}); err != nil {
		t.Fatal(err)
	}
	process.exit(terminal.ExitInfo{Code: 0})
	testwait.Until(t, log.isClosed)
	if got := log.noted(); strings.Join(got, "|") != "ローカルシェルを開きました。|セッションが終わりました。" {
		t.Fatalf("notes = %q", got)
	}
	if len(log.sizes) != 1 || log.sizes[0] != (terminal.Size{Cols: 100, Rows: 30}) {
		t.Fatalf("sizes = %v", log.sizes)
	}
}

func TestTheSecretsAStartupSnippetTypesAreKeptOutOfTheLog(t *testing.T) {
	spy := &shellOpenSpy{}
	_, log := openLogged(t, terminal.Spec{
		Kind: terminal.KindSSH, Alias: "web", Title: "web", Open: spy.open,
		Startup: func() terminal.Startup {
			return terminal.Startup{Steps: []terminal.StartupStep{{Send: "login s3cret", Enter: true}}, Secrets: []string{"s3cret"}}
		},
	})
	spy.at(0).finishOpen(nil)
	testwait.Until(t, func() bool { return spy.at(0).keystrokes() == "login s3cret\r" })
	log.mutex.Lock()
	secrets := append([]string(nil), log.secrets...)
	log.mutex.Unlock()
	if len(secrets) != 1 || secrets[0] != "s3cret" {
		t.Fatalf("secrets kept out of the log = %q", secrets)
	}
	spy.at(0).exit(terminal.ExitInfo{Code: 0})
}

func TestASessionWhoseLogCouldNotOpenSaysSoAndRunsWithoutOne(t *testing.T) {
	registry, _ := newFastRegistry()
	registry.OpenLog = func(terminal.LogTarget) (terminal.OutputLog, error) { return nil, context.DeadlineExceeded }
	process := newFakeProcess()
	session, err := registry.Open(context.Background(), terminal.Spec{
		Kind: terminal.KindShell, Title: "bash",
		Open: func(context.Context, terminal.Size) (terminal.Process, error) { return process, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(snapshotOf(session)); !strings.Contains(got, "[sshc] セッションのログを開けませんでした") {
		t.Fatalf("terminal = %q", got)
	}
	process.exit(terminal.ExitInfo{Code: 0})
}

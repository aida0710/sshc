package sshclient_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
)

// 端末の無い接続（SFTP のタブ）でも、接続を見ている人が答えられるなら、known_hosts に無い
// ホスト鍵をターミナルと同じ決まりで尋ねる。

// watchingPerson は、接続を見ている人である。尋ねられた問いと、そのときの段階を覚え、
// 決めておいた答えを返す。
type watchingPerson struct {
	accept bool

	mutex     sync.Mutex
	questions []string
	// offered は、尋ねられたときに Progress が添えていた鍵である。
	offered []terminal.OfferedHostKey
	last    terminal.ConnectionProgress
}

func (person *watchingPerson) observe(ctx context.Context) context.Context {
	return sshclient.WithConnectionObserver(ctx, sshclient.ConnectionObserver{
		Progress: func(progress terminal.ConnectionProgress) {
			person.mutex.Lock()
			defer person.mutex.Unlock()
			person.last = progress
		},
		ConfirmHostKey: func(_ context.Context, question string) (bool, error) {
			person.mutex.Lock()
			defer person.mutex.Unlock()
			person.questions = append(person.questions, question)
			if person.last.HostKey != nil {
				person.offered = append(person.offered, *person.last.HostKey)
			}
			return person.accept, nil
		},
	})
}

func (person *watchingPerson) asked() []string {
	person.mutex.Lock()
	defer person.mutex.Unlock()
	return append([]string(nil), person.questions...)
}

// unknownHostDialer は、known_hosts が空のダイアラーと、書かれた鍵の記録を返す。
func unknownHostDialer(contents []byte, knownHosts string) (sshclient.Dialer, *[]knownhosts.Candidate) {
	var written []knownhosts.Candidate
	return sshclient.Dialer{
		Auth: sshclient.Auth{ReadFile: func(string) ([]byte, error) { return contents, nil }},
		HostKeys: sshclient.HostKeys{
			Read: func(string) ([]byte, error) { return []byte(knownHosts), nil },
			Add: func(_ string, candidate knownhosts.Candidate) error {
				written = append(written, candidate)
				return nil
			},
		},
	}, &written
}

func TestSubsystemConnectionAsksTheWatchingPersonAboutAnUnknownHostAndRemembersAnAcceptedKey(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	dialer, written := unknownHostDialer(contents, "")
	person := &watchingPerson{accept: true}

	connection, err := dialer.Connect(person.observe(context.Background()), targetWith(server, path))
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	_ = connection.Close()

	fingerprint := ssh.FingerprintSHA256(server.HostKey.PublicKey())
	asked := person.asked()
	if len(asked) != 1 || !strings.Contains(asked[0], fingerprint) {
		t.Fatalf("asked %q, want one question about %s", asked, fingerprint)
	}
	// The screen shows the key from the progress, so it must be the one asked about.
	if len(person.offered) != 1 || person.offered[0].Fingerprint != fingerprint {
		t.Errorf("the progress offered %+v while asking about %s", person.offered, fingerprint)
	}
	if len(*written) != 1 || (*written)[0].Fingerprint != fingerprint {
		t.Errorf("known_hosts got %+v, want the accepted key", *written)
	}
}

func TestSubsystemConnectionDoesNotConnectWhenThePersonDeclinesTheKey(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	dialer, written := unknownHostDialer(contents, "")
	person := &watchingPerson{accept: false}

	connection, err := dialer.Connect(person.observe(context.Background()), targetWith(server, path))
	if connection != nil {
		_ = connection.Close()
		t.Fatal("a declined host returned an authenticated transport")
	}
	if !errors.Is(err, sshclient.ErrHostKeyDeclined) {
		t.Fatalf("Connect = %v, want ErrHostKeyDeclined", err)
	}
	if len(*written) != 0 {
		t.Errorf("a declined key was written to known_hosts: %+v", *written)
	}
}

// 鍵が変わった接続先は、ターミナルと同じく尋ねずに断る。尋ねること自体が攻撃の成立条件になる。
func TestSubsystemConnectionRefusesAChangedHostKeyWithoutAsking(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	other := newHostKey(t).PublicKey()
	dialer, written := unknownHostDialer(contents, knownHostsLine("["+server.Host()+"]:"+server.Port(), other))
	person := &watchingPerson{accept: true}

	connection, err := dialer.Connect(person.observe(context.Background()), targetWith(server, path))
	if connection != nil {
		_ = connection.Close()
		t.Fatal("a host with a changed key returned an authenticated transport")
	}
	if !errors.Is(err, sshclient.ErrHostKeyChanged) {
		t.Fatalf("Connect = %v, want ErrHostKeyChanged", err)
	}
	if asked := person.asked(); len(asked) != 0 || len(*written) != 0 {
		t.Errorf("a changed key was asked about (%q) or written (%+v)", asked, *written)
	}
}

// StrictHostKeyChecking yes の接続先は、ターミナルと同じく尋ねずに断る。
func TestSubsystemConnectionFollowsStrictHostKeyCheckingLikeTheTerminal(t *testing.T) {
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	dialer, written := unknownHostDialer(contents, "")
	person := &watchingPerson{accept: true}
	target := targetWith(server, path)
	target.Strict = "yes"

	_, err := dialer.Connect(person.observe(context.Background()), target)
	if !errors.Is(err, sshclient.ErrHostKeyUnknown) || errors.Is(err, sshclient.ErrHostKeyDeclined) {
		t.Fatalf("Connect = %v, want ErrHostKeyUnknown without a question", err)
	}
	if asked := person.asked(); len(asked) != 0 || len(*written) != 0 {
		t.Errorf("StrictHostKeyChecking yes asked (%q) or wrote (%+v)", asked, *written)
	}
}

// ホスト鍵に答えられる人がいても、認証の入力は尋ねない。SFTP は保存済みの資格情報だけで繋ぐ。
func TestSubsystemConnectionThatCanAskAboutAHostKeyStillAsksForNoPassword(t *testing.T) {
	server := newTestServer(t, serverOptions{Password: "hunter2"})
	dialer := dialerFor(t, server, sshclient.Auth{})
	dialer.Verbosity = func() connectionlog.Level { return connectionlog.Detailed }
	log := &lockedLog{}
	ctx := sshclient.WithConnectionObserver(context.Background(), sshclient.ConnectionObserver{
		Log: log,
		ConfirmHostKey: func(context.Context, string) (bool, error) {
			t.Error("a known host was asked about")
			return false, nil
		},
	})

	connection, err := dialer.Connect(ctx, targetWith(server))
	if connection != nil {
		_ = connection.Close()
		t.Fatal("a password the person was never asked for authenticated")
	}
	if err == nil {
		t.Fatal("Connect succeeded without a password")
	}
	if strings.Contains(log.String(), "パスワードの入力を求めます") || !strings.Contains(log.String(), "passwordは試しません：非対話") {
		t.Errorf("log = %q, want password left out as on a non-interactive connection", log.String())
	}
}

// 尋ねているあいだは ConnectTimeout を数えない。ターミナルと同じく、人は考えてから答える。
func TestSubsystemConnectionWaitsForTheAnswerLongerThanTheConnectTimeout(t *testing.T) {
	const connectTimeout = 300 * time.Millisecond
	const thinkingTime = 2 * connectTimeout
	path, contents, public := keyPair(t)
	server := newTestServer(t, serverOptions{AcceptKeys: []ssh.PublicKey{public}})
	dialer, _ := unknownHostDialer(contents, "")
	target := targetWith(server, path)
	target.Timeout = connectTimeout
	ctx := sshclient.WithConnectionObserver(context.Background(), sshclient.ConnectionObserver{
		ConfirmHostKey: func(context.Context, string) (bool, error) {
			time.Sleep(thinkingTime)
			return true, nil
		},
	})

	connection, err := dialer.Connect(ctx, target)
	if err != nil {
		t.Fatalf("Connect = %v", err)
	}
	_ = connection.Close()
}

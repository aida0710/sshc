package httpserver

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"

	"sshc/internal/api"
	"sshc/internal/connectionattempt"
	"sshc/internal/knownhosts"
	sshcSFTP "sshc/internal/sftp"
	"sshc/internal/sshclient"
	"sshc/internal/terminal"
	"sshc/internal/testwait"
)

// SFTP のタブが初めての接続先を開くと、ターミナルと同じホスト鍵の問いがタブの中に出て、
// 答えると一覧が開く。

// startHandshakeOnlyServer は、誰でも認証なしで通す SSH サーバーを立て、その宛先とホスト鍵を
// 返す。SFTP は話さない。ここで見るのは、ホスト鍵の照合で接続がどうなるかだけである。
func startHandshakeOnlyServer(t *testing.T) (string, ssh.PublicKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = connection.Close() })
			go func() {
				_, channels, requests, err := ssh.NewServerConn(connection, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					_ = channel.Reject(ssh.Prohibited, "this server only shakes hands")
				}
			}()
		}
	}()
	return listener.Addr().String(), signer.PublicKey()
}

// firstConnectionSFTP は、known_hosts が空のまま、address の SSH サーバーへ実際に接続する SFTP の
// 口を返す。接続できたら、一覧は remote から読む。written には known_hosts へ書いた鍵が入る。
func firstConnectionSFTP(t *testing.T, address string) (send func(method, target string, body []byte) (int, []byte), written func() []knownhosts.Candidate) {
	t.Helper()
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	var mutex sync.Mutex
	var remembered []knownhosts.Candidate
	dialer := sshclient.Dialer{HostKeys: sshclient.HostKeys{
		Read: func(string) ([]byte, error) { return nil, nil },
		Add: func(_ string, candidate knownhosts.Candidate) error {
			mutex.Lock()
			defer mutex.Unlock()
			remembered = append(remembered, candidate)
			return nil
		},
	}}
	target := sshclient.Target{
		Alias: "edge", HostName: host, Port: port, User: "ops", Strict: "ask",
		Methods: sshclient.DefaultMethods(), KnownHosts: sshclient.KnownHostsFiles{User: []string{"known_hosts"}},
	}
	remote := &httpContentRemote{contents: map[string]string{"/work/notes.txt": "first\n"}}
	handlers := SFTPHandlers{
		Service: &sshcSFTP.Service{Open: func(ctx context.Context, _ string) (sshcSFTP.Remote, error) {
			connection, err := dialer.Connect(ctx, target)
			if err != nil {
				return nil, err
			}
			t.Cleanup(func() { _ = connection.Close() })
			return remote, nil
		}},
		Attempts: connectionattempt.NewRegistry(),
	}
	engine, credentials := sftpHTTPEngine(t, handlers)
	send = func(method, target string, body []byte) (int, []byte) {
		response := sendKeyRequest(t, engine, credentials, method, target, body, "")
		return response.Code, response.Body.Bytes()
	}
	written = func() []knownhosts.Candidate {
		mutex.Lock()
		defer mutex.Unlock()
		return append([]knownhosts.Candidate(nil), remembered...)
	}
	return send, written
}

type listingResult struct {
	status int
	body   []byte
}

// listWithAttempt は、タブが最初の一覧を取るのと同じ要求を別の goroutine で送る。要求は接続が
// 終わるまで返らない。
func listWithAttempt(send func(method, target string, body []byte) (int, []byte)) <-chan listingResult {
	listed := make(chan listingResult, 1)
	go func() {
		status, body := send(http.MethodGet, "/api/v1/sftp/edge/entries?path=/work&attempt="+attemptTestID, nil)
		listed <- listingResult{status: status, body: body}
	}()
	return listed
}

// awaitHostKeyQuestion は、試行が尋ねている問いを、タブと同じく試行を読み直して待つ。
func awaitHostKeyQuestion(t *testing.T, send func(method, target string, body []byte) (int, []byte)) api.TerminalHostKeyQuestion {
	t.Helper()
	var question *api.TerminalHostKeyQuestion
	if !testwait.Reached(func() bool {
		status, body := send(http.MethodGet, "/api/v1/sftp/edge/connection-attempts/"+attemptTestID, nil)
		var attempt api.SFTPConnectionAttempt
		if status != http.StatusOK || json.Unmarshal(body, &attempt) != nil {
			return false
		}
		question = attempt.HostKeyQuestion
		return question != nil
	}) {
		t.Fatal("the attempt never asked about the host key")
	}
	return *question
}

func answerHostKey(send func(method, target string, body []byte) (int, []byte), fingerprint string, accept bool) (int, []byte) {
	body := fmt.Sprintf(`{"fingerprint":%q,"accept":%t}`, fingerprint, accept)
	return send(http.MethodPost, "/api/v1/sftp/edge/connection-attempts/"+attemptTestID+"/host-key", []byte(body))
}

func TestAFirstSFTPConnectionAsksAboutTheHostKeyAndListsOnceItIsAccepted(t *testing.T) {
	address, hostKey := startHandshakeOnlyServer(t)
	send, written := firstConnectionSFTP(t, address)
	listed := listWithAttempt(send)

	question := awaitHostKeyQuestion(t, send)
	fingerprint := ssh.FingerprintSHA256(hostKey)
	if question.Alias != "edge" || question.KeyType != ssh.KeyAlgoED25519 || question.Fingerprint != fingerprint {
		t.Errorf("question = %+v, want edge's %s key %s", question, ssh.KeyAlgoED25519, fingerprint)
	}
	status, body := answerHostKey(send, fingerprint, true)
	var answered api.SFTPConnectionAttempt
	if status != http.StatusOK || json.Unmarshal(body, &answered) != nil || answered.HostKeyQuestion != nil {
		t.Fatalf("answer = %d %s, want the attempt without the question", status, body)
	}

	result := <-listed
	if result.status != http.StatusOK {
		t.Fatalf("listing = %d %s", result.status, result.body)
	}
	if remembered := written(); len(remembered) != 1 || remembered[0].Fingerprint != fingerprint {
		t.Errorf("known_hosts got %+v, want the accepted key", remembered)
	}
}

func TestAFirstSFTPConnectionWhoseHostKeyIsDeclinedSaysSo(t *testing.T) {
	address, hostKey := startHandshakeOnlyServer(t)
	send, written := firstConnectionSFTP(t, address)
	listed := listWithAttempt(send)

	awaitHostKeyQuestion(t, send)
	if status, body := answerHostKey(send, ssh.FingerprintSHA256(hostKey), false); status != http.StatusOK {
		t.Fatalf("answer = %d %s", status, body)
	}

	result := <-listed
	var problem api.Problem
	_ = json.Unmarshal(result.body, &problem)
	if result.status != http.StatusBadGateway || problem.Code != "host_key_declined" {
		t.Errorf("listing = %d %s, want 502 host_key_declined", result.status, result.body)
	}
	if remembered := written(); len(remembered) != 0 {
		t.Errorf("a declined key was written: %+v", remembered)
	}
}

// 一覧の要求の外（転送など）では尋ねる相手がいないので、未知の鍵を今までどおり断る。
func TestAnSFTPConnectionNobodyWatchesRefusesAnUnknownHostKey(t *testing.T) {
	address, _ := startHandshakeOnlyServer(t)
	send, written := firstConnectionSFTP(t, address)

	status, body := send(http.MethodGet, "/api/v1/sftp/edge/entries?path=/work", nil)
	var problem api.Problem
	_ = json.Unmarshal(body, &problem)
	if status != http.StatusBadGateway || problem.Code != "host_key_unknown" {
		t.Errorf("listing = %d %s, want 502 host_key_unknown", status, body)
	}
	if remembered := written(); len(remembered) != 0 {
		t.Errorf("an unwatched connection wrote a key: %+v", remembered)
	}
}

func TestAnAnswerTheAttemptCannotTakeIsRefused(t *testing.T) {
	registry := connectionattempt.NewRegistry()
	handlers := SFTPHandlers{Service: &sshcSFTP.Service{}, Attempts: registry}
	engine, credentials := sftpHTTPEngine(t, handlers)
	send := func(target string, body string) (int, string) {
		response := sendKeyRequest(t, engine, credentials, http.MethodPost, target, []byte(body), "")
		var problem api.Problem
		_ = json.Unmarshal(response.Body.Bytes(), &problem)
		return response.Code, problem.Code
	}
	path := "/api/v1/sftp/edge/connection-attempts/" + attemptTestID + "/host-key"
	answer := `{"fingerprint":"SHA256:ZncdGvxXvUJOYNkbZuT4Ew+KkguCa24nYGfahfgoh+E","accept":true}`

	if status, code := send(path, answer); status != http.StatusNotFound || code != "sftp_connection_attempt_not_found" {
		t.Errorf("an attempt not begun = %d %s", status, code)
	}
	attempt, err := registry.Begin("edge", attemptTestID)
	if err != nil {
		t.Fatal(err)
	}
	if status, code := send(path, answer); status != http.StatusConflict || code != "sftp_no_host_key_question" {
		t.Errorf("an attempt asking nothing = %d %s", status, code)
	}
	attempt.Progress(terminal.ConnectionProgress{Phase: terminal.ConnectionHostKey, Alias: "edge", Hop: 1, Hops: 1})
	// An answer that leaves out accept is not read as a refusal.
	for _, malformed := range []string{`{"fingerprint":"SHA256:x"}`, `{"accept":true}`, `{"fingerprint":"SHA256:x","accept":"yes"}`} {
		if status, code := send(path, malformed); status != http.StatusBadRequest || code != "invalid_request" {
			t.Errorf("%s = %d %s, want 400 invalid_request", malformed, status, code)
		}
	}
}

func TestAnSFTPListingRefusedForItsHostKeyUsesTheTerminalsWords(t *testing.T) {
	tests := []struct {
		err  error
		code string
	}{
		{err: fmt.Errorf("ssh: handshake failed: %w", sshclient.ErrHostKeyDeclined), code: "host_key_declined"},
		{err: fmt.Errorf("ssh: handshake failed: %w", sshclient.ErrHostKeyUnknown), code: "host_key_unknown"},
		{err: fmt.Errorf("ssh: handshake failed: %w", sshclient.ErrHostKeyChanged), code: "host_key_changed"},
		{err: fmt.Errorf("ssh: handshake failed: %w", sshclient.ErrHostKeyRevoked), code: "host_key_revoked"},
		{err: fmt.Errorf("ssh: handshake failed: %w", &sshclient.KnownHostsSymlinkError{Path: "/home/ops/.ssh/known_hosts", Err: errors.New("link")}), code: "known_hosts_symlink"},
	}
	for _, test := range tests {
		response := serveSFTPProblem(test.err)
		var problem api.Problem
		_ = json.Unmarshal(response.Body.Bytes(), &problem)
		if response.Code != http.StatusBadGateway || problem.Code != test.code {
			t.Errorf("%v = %d %s, want 502 %s", test.err, response.Code, response.Body.String(), test.code)
		}
	}
}

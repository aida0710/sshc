package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sshc/internal/sshclient"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"sshc/internal/handoff"
	"sshc/internal/httpserver"
	"sshc/internal/platform"
)

func TestRunUsesRandomIPv4LoopbackAndReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opened := make(chan string, 1)
	var gotNetwork, gotAddress string
	dependencies := Dependencies{
		Random: bytes.NewReader(bytes.Repeat([]byte{0x81}, 96)),
		Announce: func(readiness Readiness) error {
			opened <- readiness.Entrance
			return nil
		},
		Listen: func(network, address string) (net.Listener, error) {
			gotNetwork, gotAddress = network, address
			return net.Listen(network, address)
		},
		UI:     fstest.MapFS{"index.html": {Data: []byte("ok")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:   t.TempDir(),
		Owner:  handoff.OwnerEngine,
		PID:    4242,
	}

	done := make(chan error, 1)
	go func() { done <- Run(ctx, dependencies, "test") }()

	target := <-opened
	if gotNetwork != "tcp4" {
		t.Fatalf("listen network = %s, want tcp4", gotNetwork)
	}
	host, port, err := net.SplitHostPort(gotAddress)
	if err != nil {
		t.Fatal(err)
	}
	number, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" || number < LowestPort || number > HighestPort {
		t.Fatalf("listen = %s, want 127.0.0.1 in %d..%d", gotAddress, LowestPort, HighestPort)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Hostname() != "127.0.0.1" || parsed.RawQuery != "" || !strings.HasPrefix(parsed.Fragment, "bootstrap=") {
		t.Fatalf("target = %q", target)
	}
	if got := strings.TrimPrefix(parsed.Fragment, "bootstrap="); len(got) != 43 {
		t.Fatalf("bootstrap length = %d, want 43", len(got))
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run = %v", err)
	}
}

func TestRunLeavesAReplacementHandoffOwnedByAnotherSecret(t *testing.T) {
	home := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	replacementSecret := "a later engine's distinct secret"
	dependencies := Dependencies{
		Random: bytes.NewReader(bytes.Repeat([]byte{0x82}, 96)),
		Announce: func(readiness Readiness) error {
			base, _, found := strings.Cut(readiness.Entrance, "/#")
			if !found {
				return errors.New("missing bootstrap target")
			}
			err := handoff.Write(HandoffDir(home), handoff.Handoff{
				SchemaVersion:   handoff.SchemaVersion,
				URL:             base,
				Secret:          replacementSecret,
				Owner:           handoff.OwnerEngine,
				PID:             4243,
				Version:         "another-test",
				ProtocolVersion: handoff.ProtocolVersion,
			})
			cancel()
			return err
		},
		Listen: net.Listen,
		UI:     fstest.MapFS{"index.html": {Data: []byte("ok")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:   home,
		Owner:  handoff.OwnerEngine,
		PID:    4242,
	}

	if err := Run(ctx, dependencies, "test"); err != nil {
		t.Fatalf("Run = %v", err)
	}
	document, err := handoff.Read(HandoffDir(home))
	if err != nil {
		t.Fatalf("Read replacement handoff = %v", err)
	}
	if document.Secret != replacementSecret {
		t.Errorf("remaining secret = %q, want replacement", document.Secret)
	}
}

var errAccept = errors.New("accept failed")

type failingListener struct {
	failed chan struct{}
}

func (listener *failingListener) Accept() (net.Conn, error) {
	listener.failed <- struct{}{}
	return nil, errAccept
}
func (*failingListener) Close() error { return nil }
func (*failingListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: 43123}
}

func TestRunReturnsServerFailureWithoutWaitingForCancellation(t *testing.T) {
	listener := &failingListener{failed: make(chan struct{}, 1)}
	dependencies := Dependencies{
		Random:   bytes.NewReader(bytes.Repeat([]byte{0x91}, 96)),
		Announce: func(Readiness) error { return nil },
		Listen:   func(string, string) (net.Listener, error) { return listener, nil },
		UI:       fstest.MapFS{"index.html": {Data: []byte("ok")}},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:     t.TempDir(),
		Owner:    handoff.OwnerEngine,
		PID:      4242,
	}

	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), dependencies, "test") }()

	// Accept の失敗を同期点にする。起動時間を含む wall clock と競争させず、
	// context を取り消していない状態でその失敗が Run から返ることを確かめる。
	<-listener.failed
	if err := <-done; !errors.Is(err, errAccept) {
		t.Fatalf("Run error = %v", err)
	}
}

func TestRunShutsServerDownWhenTheEntranceCannotBeAnnounced(t *testing.T) {
	announceErr := errors.New("browser unavailable")
	listener := &trackingListener{Listener: mustListen(t)}
	dependencies := Dependencies{
		Random:   bytes.NewReader(bytes.Repeat([]byte{0x72}, 96)),
		Announce: func(Readiness) error { return announceErr },
		Listen:   func(string, string) (net.Listener, error) { return listener, nil },
		UI:       fstest.MapFS{"index.html": {Data: []byte("ok")}},
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:     t.TempDir(),
		Owner:    handoff.OwnerEngine,
		PID:      4242,
	}

	err := Run(context.Background(), dependencies, "test")
	if !errors.Is(err, announceErr) {
		t.Fatalf("Run error = %v", err)
	}
	if !listener.closed.Load() {
		t.Fatal("listener was not closed after browser failure")
	}
}

type stubToolchain struct{}

func (stubToolchain) KeyGen() (string, error) { return "/usr/bin/ssh-keygen", nil }

type stubKeyAgent struct{}

func (stubKeyAgent) Available(context.Context) bool { return false }
func (stubKeyAgent) List(context.Context) ([]platform.AgentIdentity, error) {
	return nil, platform.ErrAgentUnavailable
}
func (stubKeyAgent) Add(context.Context, platform.AgentAddRequest) error {
	return platform.ErrAgentUnavailable
}
func (stubKeyAgent) Remove(context.Context, string) error { return platform.ErrAgentUnavailable }

type keyVaultSession struct {
	base    string
	client  *http.Client
	cookie  *http.Cookie
	csrf    string
	testing *testing.T
}

func (call keyVaultSession) do(method, path string, body []byte, headers map[string]string) *http.Response {
	call.testing.Helper()
	request, err := http.NewRequest(method, call.base+path, bytes.NewReader(body))
	if err != nil {
		call.testing.Fatalf("build %s %s: %v", method, path, err)
	}
	request.AddCookie(call.cookie)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	request.Header.Set("X-SSHC-CSRF", call.csrf)
	if method != http.MethodGet {
		request.Header.Set("Origin", call.base)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := call.client.Do(request)
	if err != nil {
		call.testing.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}

func TestRunExposesTheKeyVaultAndItsTrashThroughTheWiredProcess(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatalf("create ssh directory: %v", err)
	}
	opened := make(chan string, 1)
	dependencies := Dependencies{
		Random: rand.Reader,
		Announce: func(readiness Readiness) error {
			opened <- readiness.Entrance
			return nil
		},
		Listen:    net.Listen,
		UI:        fstest.MapFS{"index.html": {Data: []byte("ok")}},
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:      home,
		Owner:     handoff.OwnerEngine,
		PID:       4242,
		Toolchain: stubToolchain{},
		KeyAgent:  stubKeyAgent{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, dependencies, "test") }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run error = %v", err)
		}
	}()

	target := <-opened
	base, fragment, _ := strings.Cut(target, "/#")
	bootstrapToken := strings.TrimPrefix(fragment, "bootstrap=")

	client := &http.Client{}
	bootstrapRequest, err := http.NewRequest(http.MethodPost, base+"/api/v1/session/bootstrap", nil)
	if err != nil {
		t.Fatalf("build bootstrap request: %v", err)
	}
	bootstrapRequest.Header.Set("Origin", base)
	bootstrapRequest.Header.Set("Sec-Fetch-Site", "same-origin")
	bootstrapRequest.Header.Set("X-SSHC-Bootstrap", bootstrapToken)
	bootstrapResponse, err := client.Do(bootstrapRequest)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	var bootstrapBody struct {
		CsrfToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(bootstrapResponse.Body).Decode(&bootstrapBody); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	cookies := bootstrapResponse.Cookies()
	bootstrapResponse.Body.Close()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %#v", cookies)
	}
	call := keyVaultSession{base: base, client: client, cookie: cookies[0], csrf: bootstrapBody.CsrfToken, testing: t}

	unlocked := call.do(http.MethodPost, "/api/v1/passwords/initialise",
		[]byte(`{"passphrase":"a master password for this run"}`), nil)
	if unlocked.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(unlocked.Body)
		t.Fatalf("initialise the vault = %d: %s", unlocked.StatusCode, body)
	}
	unlocked.Body.Close()

	listing := call.do(http.MethodGet, "/api/v1/keys", nil, nil)
	defer listing.Body.Close()
	if listing.StatusCode != http.StatusOK {
		t.Fatalf("list keys = %d, want 200", listing.StatusCode)
	}
	if got := listing.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}

	generateBody := []byte(`{"algorithm":"ed25519","fileName":"id_work","comment":"aida@laptop","passphrase":"correct horse","unencrypted":false}`)
	generated := call.do(http.MethodPost, "/api/v1/keys", generateBody, nil)
	var generateResult struct {
		Id string `json:"id"`
	}
	if err := json.NewDecoder(generated.Body).Decode(&generateResult); err != nil {
		t.Fatalf("decode generate: %v", err)
	}
	generated.Body.Close()
	if generated.StatusCode != http.StatusCreated || generateResult.Id == "" {
		t.Fatalf("generate = %d %#v", generated.StatusCode, generateResult)
	}

	trashed := call.do(http.MethodPost, "/api/v1/keys/"+generateResult.Id+"/trash", nil, nil)
	body, _ := io.ReadAll(trashed.Body)
	trashed.Body.Close()
	if trashed.StatusCode != http.StatusOK {
		t.Fatalf("trash = %d, want 200: %s", trashed.StatusCode, body)
	}
	if _, statErr := os.Lstat(filepath.Join(home, ".ssh", "id_work")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("the key is still in the workspace: %v", statErr)
	}

	overview := call.do(http.MethodGet, "/api/v1/config/overview", nil, nil)
	overview.Body.Close()
	if overview.StatusCode != http.StatusOK {
		t.Fatalf("config overview = %d, want 200", overview.StatusCode)
	}
}

func TestBuildReturnsAServerAndAOneTimeBootstrapToken(t *testing.T) {
	home := t.TempDir()
	server, bootstrap, err := Build(Dependencies{
		Home:   home,
		Random: bytes.NewReader(bytes.Repeat([]byte{0x24}, 512)),
		Listen: net.Listen,
		UI:     fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Owner:  handoff.OwnerEngine,
		PID:    4242,
	}, "build-test")
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}
	if len(bootstrap) != 43 {
		t.Fatalf("bootstrap length = %d, want 43", len(bootstrap))
	}
	if !strings.HasPrefix(server.URL(), "http://127.0.0.1:") {
		t.Fatalf("URL() = %q", server.URL())
	}
	if len(server.Routes()) == 0 {
		t.Fatal("Build() produced a server with no routes")
	}

	served := make(chan error, 1)
	go func() { served <- server.Serve() }()
	server.BeginStopping()
	server.BeginShutdown()
	if err := <-served; err != nil {
		t.Fatalf("Serve() = %v", err)
	}
	if err := server.Wait(); err != nil {
		t.Fatalf("Wait() = %v", err)
	}
}

func TestBuildMarksLoopbackListenerFailures(t *testing.T) {
	denied := errors.New("socket denied")
	_, _, err := Build(Dependencies{
		Home:   t.TempDir(),
		Random: bytes.NewReader(bytes.Repeat([]byte{0x26}, 512)),
		Listen: func(string, string) (net.Listener, error) { return nil, denied },
		UI:     fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Owner:  handoff.OwnerEngine,
		PID:    4242,
	}, "build-test")
	if !errors.Is(err, ErrListen) {
		t.Fatalf("Build() = %v, want ErrListen", err)
	}
}

func TestBuildWritesAVersionedOwnedHandoff(t *testing.T) {
	home := t.TempDir()
	server, _, err := Build(Dependencies{
		Home:   home,
		Random: bytes.NewReader(bytes.Repeat([]byte{0x25}, 512)),
		Listen: net.Listen,
		UI:     fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Owner:  handoff.OwnerEngine,
		PID:    777,
	}, "v1.2.3")
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}

	document, err := handoff.Read(HandoffDir(home))
	if err != nil {
		t.Fatalf("Read handoff = %v", err)
	}
	if document.SchemaVersion != handoff.SchemaVersion || document.ProtocolVersion != handoff.ProtocolVersion {
		t.Errorf("versions = schema %d protocol %d", document.SchemaVersion, document.ProtocolVersion)
	}
	if document.URL != server.URL() || document.Owner != handoff.OwnerEngine || document.PID != 777 || document.Version != "v1.2.3" {
		t.Errorf("document = %#v", document)
	}

	served := make(chan error, 1)
	go func() { served <- server.Serve() }()
	stop := func() {
		server.BeginStopping()
		server.BeginShutdown()
	}
	request, err := http.NewRequest(http.MethodGet, server.URL()+httpserver.StatusPath, nil)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	request.Header.Set(handoff.HeaderName, document.Secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	var status httpserver.CLIStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		response.Body.Close()
		stop()
		t.Fatal(err)
	}
	response.Body.Close()
	if status.Owner != document.Owner || status.Version != document.Version || status.ProtocolVersion != document.ProtocolVersion {
		t.Errorf("status identity = %#v, handoff = %#v", status, document)
	}
	stop()
	if err := <-served; err != nil {
		t.Fatalf("Serve() = %v", err)
	}
}

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

type trackingListener struct {
	net.Listener
	closed atomic.Bool
}

func (listener *trackingListener) Close() error {
	listener.closed.Store(true)
	return listener.Listener.Close()
}

func TestConnectionAliasesCarryTheJumpChainBeforeTheDestination(t *testing.T) {
	listed := appendAliases(nil, sshclient.Target{
		Alias: "far",
		Jump: []sshclient.Target{{
			Alias: "edge",
			Jump:  []sshclient.Target{{Alias: "outer"}},
		}},
	})

	want := []string{"outer", "edge", "far"}
	if len(listed) != len(want) {
		t.Fatalf("aliases = %#v, want %#v", listed, want)
	}
	for index := range want {
		if listed[index] != want[index] {
			t.Fatalf("aliases = %#v, want %#v", listed, want)
		}
	}
}

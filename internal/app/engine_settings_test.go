package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"sshc/internal/application"
	"sshc/internal/browserauth"
	"sshc/internal/handoff"
	"sshc/internal/platform/windowsacl/acltest"
	"sshc/internal/secret"
	"sshc/internal/storage"
)

// writeMetadata は、metadata.json をそのままのバイト列で置く。同期で届いたものと、
// 前のバージョンの sshc が書いたものを作る。
func writeMetadata(t *testing.T, home, contents string) {
	t.Helper()
	acltest.WritePrivateFile(t, filepath.Join(home, ".ssh", "sshc", application.MetadataFileName), []byte(contents))
}

// portReporter は、空いているポートで待ち受けながら、要求されたポートで待ち受けて
// いるように見せる。利用者の sshc が使う既定のポートを、テストで取らないためにある。
type portReporter struct {
	net.Listener
	port int
}

func (listener portReporter) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IP{127, 0, 0, 1}, Port: listener.port}
}

func TestAMachineUpdatedFromAnOlderVersionKeepsItsEngineSettings(t *testing.T) {
	home := t.TempDir()
	// 前のバージョンの sshc は、sshc エンジンの設定を metadata.json の engine 節に書いていた。
	writeMetadata(t, home, `{"schemaVersion":8,"engine":{"port":43123,`+
		`"vaultAutoLock":{"mode":"idle","value":45,"unit":"minutes"}}}`)

	services, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if got := services.config.EngineSettings().Port; got != 43123 {
		t.Fatalf("port = %d, want the port saved before the update", got)
	}
	if got := services.vault.IdleTimeout(); got != 45*time.Minute {
		t.Fatalf("IdleTimeout = %v, want the auto-lock saved before the update", got)
	}

	// metadata.json を今の形で保存し直しても、このマシンの設定は残る。設定の保存は、
	// 前の内容の控えを Vault の鍵で封じるので、Vault が要る。
	if err := services.vault.Initialise(""); err != nil {
		t.Fatal(err)
	}
	if _, err := services.config.SetFileTransferSettings(application.FileTransferSettings{MaxConcurrent: 2}); err != nil {
		t.Fatal(err)
	}
	restarted, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if got := restarted.config.EngineSettings().Port; got != 43123 {
		t.Fatalf("port after metadata.json was saved again = %d, want 43123", got)
	}
}

func TestEngineSettingsInAMetadataFromAnotherMachineDoNotChangeThisMachine(t *testing.T) {
	home := t.TempDir()
	// このバージョンで一度起動したあとに、前のバージョンのマシンが書いた metadata.json が
	// 同期で届く。
	if _, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader}); err != nil {
		t.Fatal(err)
	}
	writeMetadata(t, home, `{"schemaVersion":8,"engine":{"port":60000,"vaultAutoLock":{"mode":"restart"}}}`)

	services, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader})
	if err != nil {
		t.Fatal(err)
	}
	if got := services.config.EngineSettings(); got != (application.EngineSettings{}) {
		t.Fatalf("EngineSettings = %#v, want this machine's (none)", got)
	}
	if got := services.vault.IdleTimeout(); got != secret.IdleTimeout {
		t.Fatalf("IdleTimeout = %v, want the default %v", got, secret.IdleTimeout)
	}
}

func TestAPortFromAnotherMachineNeitherMovesTheEngineNorRevokesTheBrowsers(t *testing.T) {
	home := t.TempDir()
	if _, err := newEngineServices(Dependencies{Home: home, Random: rand.Reader}); err != nil {
		t.Fatal(err)
	}
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		t.Fatal(err)
	}
	registrations := browserauth.NewStore(workspace, bytes.NewReader(bytes.Repeat([]byte{0x84}, 256)))
	if err := registrations.SetPort(DefaultPort); err != nil {
		t.Fatal(err)
	}
	token, issued, err := registrations.Register("")
	if err != nil || !issued {
		t.Fatalf("seed browser registration = (%q, %t, %v)", token, issued, err)
	}
	writeMetadata(t, home, `{"schemaVersion":8,"engine":{"port":60000}}`)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var asked []string
	dependencies := Dependencies{
		Random:      bytes.NewReader(bytes.Repeat([]byte{0x83}, 512)),
		DefaultPort: DefaultPort,
		Announce:    func(Readiness) error { cancel(); return nil },
		Listen: func(network, address string) (net.Listener, error) {
			asked = append(asked, address)
			_, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			number, err := strconv.Atoi(port)
			if err != nil {
				return nil, err
			}
			listener, err := net.Listen(network, "127.0.0.1:0")
			if err != nil {
				return nil, err
			}
			return portReporter{Listener: listener, port: number}, nil
		},
		UI:     fstest.MapFS{"index.html": {Data: []byte("ok")}},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Home:   home, Owner: handoff.OwnerEngine, PID: 4242,
		DockerEnvironment: environmentWithoutDocker,
	}
	if err := Run(ctx, dependencies, "test"); err != nil {
		t.Fatal(err)
	}

	want := net.JoinHostPort("127.0.0.1", strconv.Itoa(DefaultPort))
	if len(asked) != 1 || asked[0] != want {
		t.Fatalf("asked for %v, want only this machine's %s", asked, want)
	}
	if recovery, err := registrations.Recover(token); err != nil || !recovery.Accepted() {
		t.Fatalf("this machine's browser registration was revoked: accepted=%t err=%v", recovery.Accepted(), err)
	}
}

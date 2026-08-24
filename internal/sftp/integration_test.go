package sftp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	pkgsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"sshc/internal/sftp"
	"sshc/internal/sshclient"
)

type integrationRemote struct {
	sftp.Remote
	transport *sshclient.Connection
}

func (remote *integrationRemote) Close() error {
	return errors.Join(remote.Remote.Close(), remote.transport.Close())
}

func integrationService(t *testing.T) sftp.Service {
	t.Helper()
	address := os.Getenv("SSHC_TEST_SSH_ADDR")
	if address == "" {
		t.Skip("SSHC_TEST_SSH_ADDR is not set; run make integration-up and make integration")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("SSHC_TEST_SSH_ADDR=%q: %v", address, err)
	}
	identity := os.Getenv("SSHC_TEST_SSH_KEY")
	if identity == "" {
		t.Skip("SSHC_TEST_SSH_KEY is not set")
	}
	keys, err := sshclient.ScanHostKeys(t.Context(), nil, address, 0)
	if err != nil {
		t.Fatalf("scan host keys: %v", err)
	}
	var known strings.Builder
	for _, key := range keys {
		field := host
		if port != "22" {
			field = "[" + host + "]:" + port
		}
		known.WriteString(field + " " + string(ssh.MarshalAuthorizedKey(key)))
	}
	dialer := sshclient.Dialer{
		Auth: sshclient.Auth{Stored: func(candidate string) (string, bool) {
			return os.Getenv("SSHC_TEST_SSH_KEY_PASSPHRASE"), candidate == identity
		}},
		HostKeys: sshclient.HostKeys{Read: func() ([]byte, error) { return []byte(known.String()), nil }},
	}
	target := sshclient.Target{
		Alias: "integration", HostName: host, Port: port, User: os.Getenv("SSHC_TEST_SSH_USER"),
		Methods: sshclient.DefaultMethods(), Identities: []string{identity}, IdentitiesOnly: true,
		Timeout: 30 * time.Second, Strict: "yes",
	}
	return sftp.Service{Open: func(ctx context.Context, alias string) (sftp.Remote, error) {
		if alias != target.Alias {
			return nil, fmt.Errorf("unexpected alias %q", alias)
		}
		transport, err := dialer.Connect(ctx, target)
		if err != nil {
			return nil, err
		}
		client, err := pkgsftp.NewClient(transport.Client())
		if err != nil {
			_ = transport.Close()
			return nil, err
		}
		return &integrationRemote{Remote: sftp.NewClient(client), transport: transport}, nil
	}}
}

func TestServiceRoundTripsFilesAgainstOpenSSHSFTP(t *testing.T) {
	service := integrationService(t)
	root := fmt.Sprintf("/tmp/sshc-sftp-%d", time.Now().UnixNano())
	first := path.Join(root, "first.txt")
	second := path.Join(root, "payload.bin")
	renamed := path.Join(root, "renamed.bin")
	t.Cleanup(func() {
		for _, candidate := range []string{first, second, renamed} {
			_ = service.Delete(context.Background(), "integration", candidate)
		}
		_ = service.Delete(context.Background(), "integration", root)
	})

	if _, err := service.Mkdir(t.Context(), "integration", root); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := service.Upload(t.Context(), "integration", first, strings.NewReader("alpha\n"), sftp.UploadOptions{}); err != nil {
		t.Fatalf("upload text: %v", err)
	}
	if _, err := service.Upload(t.Context(), "integration", second, bytes.NewReader([]byte{0, 1, 2, 3}), sftp.UploadOptions{}); err != nil {
		t.Fatalf("upload binary: %v", err)
	}

	var downloaded bytes.Buffer
	if _, err := service.Download(t.Context(), "integration", first, &downloaded); err != nil {
		t.Fatalf("download: %v", err)
	}
	if got := downloaded.String(); got != "alpha\n" {
		t.Fatalf("downloaded = %q", got)
	}

	opened, err := service.ReadText(t.Context(), "integration", first)
	if err != nil {
		t.Fatalf("read text: %v", err)
	}
	saved, err := service.SaveText(t.Context(), "integration", first, "beta\n", opened.Revision)
	if err != nil {
		t.Fatalf("save text: %v", err)
	}
	if saved.Contents != "beta\n" {
		t.Fatalf("saved contents = %q", saved.Contents)
	}

	stale := saved.Revision
	if _, err := service.Upload(t.Context(), "integration", first, strings.NewReader("external\n"), sftp.UploadOptions{Overwrite: true}); err != nil {
		t.Fatalf("external upload: %v", err)
	}
	if _, err := service.SaveText(t.Context(), "integration", first, "mine\n", stale); !errors.Is(err, sftp.ErrConflict) {
		t.Fatalf("stale save = %v, want ErrConflict", err)
	}

	if _, err := service.Rename(t.Context(), "integration", second, renamed); err != nil {
		t.Fatalf("rename: %v", err)
	}
	entries, err := service.List(t.Context(), "integration", root)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 || entries[0].Name != "first.txt" || entries[1].Name != "renamed.bin" {
		t.Fatalf("entries = %#v", entries)
	}
	changed, err := service.Chmod(t.Context(), "integration", first, 0o640, entries[0].Revision)
	if err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if changed.Mode.Perm() != 0o640 {
		t.Fatalf("chmod mode = %o", changed.Mode.Perm())
	}
	var archived bytes.Buffer
	if _, err := service.DownloadArchive(t.Context(), "integration", root, &archived); err != nil {
		t.Fatalf("archive: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archived.Bytes()), int64(archived.Len()))
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if len(reader.File) != 3 || reader.File[0].Name != path.Base(root)+"/" {
		t.Fatalf("archive entries = %#v", reader.File)
	}

	for _, candidate := range []string{first, renamed} {
		if err := service.Delete(t.Context(), "integration", candidate); err != nil {
			t.Fatalf("delete %s: %v", candidate, err)
		}
	}
	if err := service.Delete(t.Context(), "integration", root); err != nil {
		t.Fatalf("delete directory: %v", err)
	}
}

func TestResumableTransferAgainstOpenSSHSFTP(t *testing.T) {
	service := integrationService(t)
	manager := sftp.NewTransferManager(&service)
	root := fmt.Sprintf("/tmp/sshc-sftp-resume-%d", time.Now().UnixNano())
	target := path.Join(root, "large.bin")
	cancelled := path.Join(root, "cancel.bin")
	t.Cleanup(func() {
		_ = service.Delete(context.Background(), "integration", target)
		_ = manager.Cancel(context.Background(), "integration", "cancel_transfer_123", cancelled)
		_ = service.Delete(context.Background(), "integration", root)
	})
	if _, err := service.Mkdir(t.Context(), "integration", root); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("resumable-data-"), 180_000)
	started, err := manager.Start(t.Context(), "integration", "large_transfer_123", target, sftp.StartUploadOptions{Size: int64(len(payload))})
	if err != nil {
		t.Fatal(err)
	}
	first := 1 << 20
	if _, err := manager.Append(t.Context(), "integration", started.ID, target, 0, int64(len(payload)), payload[:first]); err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.Start(t.Context(), "integration", started.ID, target, sftp.StartUploadOptions{Size: int64(len(payload)), ExpectedRevision: started.ExpectedRevision})
	if err != nil || resumed.Offset != int64(first) {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	if _, err := manager.Append(t.Context(), "integration", started.ID, target, resumed.Offset, int64(len(payload)), payload[first:]); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "integration", started.ID, target, int64(len(payload)), started.ExpectedRevision); err != nil {
		t.Fatal(err)
	}
	var tail bytes.Buffer
	if _, err := service.DownloadFrom(t.Context(), "integration", target, int64(first), &tail); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(tail.Bytes(), payload[first:]) {
		t.Fatalf("resumed download bytes = %d", tail.Len())
	}

	cancelStart, err := manager.Start(t.Context(), "integration", "cancel_transfer_123", cancelled, sftp.StartUploadOptions{Size: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Append(t.Context(), "integration", cancelStart.ID, cancelled, 0, 4, []byte("part")); err != nil {
		t.Fatal(err)
	}
	if err := manager.Cancel(t.Context(), "integration", cancelStart.ID, cancelled); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Stat(t.Context(), "integration", cancelled); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("cancelled target = %v", err)
	}
}

var _ io.Closer = (*integrationRemote)(nil)

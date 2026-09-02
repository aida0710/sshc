package sftp_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strings"
	"sync"
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
		if alias != target.Alias && alias != "integration-source" && alias != "integration-target" {
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

func TestRemoteCopyMoveAndCompareAgainstOpenSSHSFTP(t *testing.T) {
	service := integrationService(t)
	root := fmt.Sprintf("/tmp/sshc-sftp-remote-%d", time.Now().UnixNano())
	sourceRoot := path.Join(root, "source")
	targetRoot := path.Join(root, "target")
	sourceFile := path.Join(sourceRoot, "copy.txt")
	movingFile := path.Join(sourceRoot, "move.txt")
	t.Cleanup(func() {
		for _, candidate := range []string{
			path.Join(targetRoot, "copy.txt"), path.Join(targetRoot, "move.txt"),
			sourceFile, movingFile,
		} {
			_ = service.Delete(context.Background(), "integration", candidate)
		}
		_ = service.Delete(context.Background(), "integration", sourceRoot)
		_ = service.Delete(context.Background(), "integration", targetRoot)
		_ = service.Delete(context.Background(), "integration", root)
	})
	for _, directory := range []string{root, sourceRoot, targetRoot} {
		if _, err := service.Mkdir(t.Context(), "integration", directory); err != nil {
			t.Fatal(err)
		}
	}
	integrationUpload(t, &service, sourceFile, []byte("copied directly\n"), false)
	integrationUpload(t, &service, movingFile, []byte("moved directly\n"), false)

	if err := service.CopyRemote(t.Context(), sftp.RemoteTransferRequest{
		SourceAlias: "integration-source", SourcePath: sourceFile,
		TargetAlias: "integration-target", TargetPath: path.Join(targetRoot, "copy.txt"),
		Operation: sftp.RemoteCopy,
	}, nil); err != nil {
		t.Fatalf("remote copy: %v", err)
	}
	comparison, err := service.CompareDirectories(t.Context(), "integration-source", sourceRoot, "integration-target", targetRoot)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if len(comparison.Entries) != 2 {
		t.Fatalf("comparison = %#v", comparison.Entries)
	}

	if err := service.CopyRemote(t.Context(), sftp.RemoteTransferRequest{
		SourceAlias: "integration-source", SourcePath: movingFile,
		TargetAlias: "integration-target", TargetPath: path.Join(targetRoot, "move.txt"),
		Operation: sftp.RemoteMove,
	}, nil); err != nil {
		t.Fatalf("remote move: %v", err)
	}
	if _, err := service.Stat(t.Context(), "integration-source", movingFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("moved source still exists: %v", err)
	}
	var downloaded bytes.Buffer
	prepared, err := service.PrepareDownload(t.Context(), "integration-target", path.Join(targetRoot, "move.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if _, err := prepared.WriteFrom(t.Context(), 0, &downloaded); err != nil {
		t.Fatal(err)
	}
	if downloaded.String() != "moved directly\n" {
		t.Fatalf("moved contents = %q", downloaded.String())
	}
}

func integrationUpload(t *testing.T, service *sftp.Service, target string, payload []byte, overwrite bool) {
	t.Helper()
	manager := sftp.NewTransferManager(service)
	id := fmt.Sprintf("transfer_%x", time.Now().UnixNano())
	started, err := manager.Start(t.Context(), "integration", id, target, sftp.StartUploadOptions{Size: int64(len(payload)), Overwrite: overwrite})
	if err != nil {
		t.Fatalf("start fixture upload: %v", err)
	}
	if _, err := manager.Append(t.Context(), "integration", id, target, 0, int64(len(payload)), payload); err != nil {
		t.Fatalf("append fixture upload: %v", err)
	}
	fingerprint, err := sftp.SourceFingerprint(t.Context(), bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "integration", id, target, int64(len(payload)), started.ExpectedRevision, fingerprint); err != nil {
		t.Fatalf("complete fixture upload: %v", err)
	}
}

func TestListDirectoryStartsAtOpenSSHUserHome(t *testing.T) {
	service := integrationService(t)
	listing, err := service.ListDirectory(t.Context(), "integration", "")
	if err != nil {
		t.Fatalf("list initial directory: %v", err)
	}
	if listing.Path != "/config" {
		t.Fatalf("initial path = %q, want fixture user's home /config", listing.Path)
	}
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
	integrationUpload(t, &service, first, []byte("alpha\n"), false)
	integrationUpload(t, &service, second, []byte{0, 1, 2, 3}, false)

	var downloaded bytes.Buffer
	prepared, err := service.PrepareDownload(t.Context(), "integration", first)
	if err != nil {
		t.Fatalf("prepare download: %v", err)
	}
	if _, err := prepared.WriteFrom(t.Context(), 0, &downloaded); err != nil {
		t.Fatalf("download: %v", err)
	}
	_ = prepared.Close()
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
	integrationUpload(t, &service, first, []byte("external\n"), true)
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
	payload := make([]byte, (64<<20)+17)
	for index := range payload {
		payload[index] = byte((index*31 + 17) % 251)
	}
	started, err := manager.Start(t.Context(), "integration", "large_transfer_123", target, sftp.StartUploadOptions{Size: int64(len(payload))})
	if err != nil {
		t.Fatal(err)
	}
	const chunk = 1 << 20
	const resumeAfter = 7 << 20
	for offset := 0; offset < resumeAfter; offset += chunk {
		if _, err := manager.Append(t.Context(), "integration", started.ID, target, int64(offset), int64(len(payload)), payload[offset:offset+chunk]); err != nil {
			t.Fatal(err)
		}
	}
	resumed, err := manager.Start(t.Context(), "integration", started.ID, target, sftp.StartUploadOptions{Size: int64(len(payload)), ExpectedRevision: started.ExpectedRevision})
	if err != nil || resumed.Offset != int64(resumeAfter) {
		t.Fatalf("resume = %+v, %v", resumed, err)
	}
	for offset := resumeAfter; offset < len(payload); offset += chunk {
		end := min(offset+chunk, len(payload))
		if _, err := manager.Append(t.Context(), "integration", started.ID, target, int64(offset), int64(len(payload)), payload[offset:end]); err != nil {
			t.Fatal(err)
		}
	}
	fingerprint, err := sftp.SourceFingerprint(t.Context(), bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Complete(t.Context(), "integration", started.ID, target, int64(len(payload)), started.ExpectedRevision, fingerprint); err != nil {
		t.Fatal(err)
	}
	downloaded := sha256.New()
	prepared, err := service.PrepareDownload(t.Context(), "integration", target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.WriteFrom(t.Context(), 0, downloaded); err != nil {
		t.Fatal(err)
	}
	wantDigest := sha256.Sum256(payload)
	if !bytes.Equal(downloaded.Sum(nil), wantDigest[:]) {
		t.Fatalf("large fixture digest = %x, want %x", downloaded.Sum(nil), wantDigest)
	}
	tail := sha256.New()
	if _, err := prepared.WriteFrom(t.Context(), int64(resumeAfter), tail); err != nil {
		t.Fatal(err)
	}
	_ = prepared.Close()
	wantTail := sha256.Sum256(payload[resumeAfter:])
	if !bytes.Equal(tail.Sum(nil), wantTail[:]) {
		t.Fatalf("resumed download digest = %x, want %x", tail.Sum(nil), wantTail)
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

func TestConcurrentFilesAgainstOpenSSHSFTP(t *testing.T) {
	service := integrationService(t)
	manager := sftp.NewTransferManager(&service)
	root := fmt.Sprintf("/tmp/sshc-sftp-concurrent-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_ = manager.Close()
		for index := 0; index < 8; index++ {
			_ = service.Delete(context.Background(), "integration", path.Join(root, fmt.Sprintf("file-%02d.bin", index)))
		}
		_ = service.Delete(context.Background(), "integration", root)
	})
	if _, err := service.Mkdir(t.Context(), "integration", root); err != nil {
		t.Fatal(err)
	}

	const fileCount = 8
	const fileSize = 4 << 20
	payloads := make([][]byte, fileCount)
	for index := range payloads {
		payloads[index] = make([]byte, fileSize)
		for offset := range payloads[index] {
			payloads[index][offset] = byte((offset*31 + index*17) % 251)
		}
	}

	errorsFound := make(chan error, fileCount)
	var uploads sync.WaitGroup
	for index := range payloads {
		uploads.Add(1)
		go func() {
			defer uploads.Done()
			target := path.Join(root, fmt.Sprintf("file-%02d.bin", index))
			id := fmt.Sprintf("parallel_%02d", index)
			started, err := manager.Start(t.Context(), "integration", id, target, sftp.StartUploadOptions{Size: fileSize})
			if err != nil {
				errorsFound <- fmt.Errorf("start file %d: %w", index, err)
				return
			}
			for offset := 0; offset < fileSize; offset += 1 << 20 {
				end := min(offset+(1<<20), fileSize)
				if _, err := manager.Append(t.Context(), "integration", id, target, int64(offset), fileSize, payloads[index][offset:end]); err != nil {
					errorsFound <- fmt.Errorf("append file %d at %d: %w", index, offset, err)
					return
				}
			}
			fingerprint, err := sftp.SourceFingerprint(t.Context(), bytes.NewReader(payloads[index]), fileSize)
			if err != nil {
				errorsFound <- fmt.Errorf("fingerprint file %d: %w", index, err)
				return
			}
			if _, err := manager.Complete(t.Context(), "integration", id, target, fileSize, started.ExpectedRevision, fingerprint); err != nil {
				errorsFound <- fmt.Errorf("complete file %d: %w", index, err)
			}
		}()
	}
	uploads.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}
	if t.Failed() {
		return
	}

	for index, payload := range payloads {
		target := path.Join(root, fmt.Sprintf("file-%02d.bin", index))
		prepared, err := service.PrepareDownload(t.Context(), "integration", target)
		if err != nil {
			t.Fatalf("prepare file %d: %v", index, err)
		}
		digest := sha256.New()
		_, copyErr := prepared.WriteFrom(t.Context(), 0, digest)
		closeErr := prepared.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			t.Fatalf("download file %d: %v", index, err)
		}
		want := sha256.Sum256(payload)
		if !bytes.Equal(digest.Sum(nil), want[:]) {
			t.Fatalf("file %d digest = %x, want %x", index, digest.Sum(nil), want)
		}
	}
}

var _ io.Closer = (*integrationRemote)(nil)

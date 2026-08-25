package sftp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPreparedSpoolQuotaLivesUntilTheLastCloneCloses(t *testing.T) {
	if downloadSpoolDirectory() == "" {
		t.Fatal("download spool directory unavailable")
	}
	temporary, err := os.CreateTemp(t.TempDir(), "lease-*.part")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := temporary.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	processSpoolMu.Lock()
	original := processSpoolBytes
	processSpoolMu.Unlock()
	if err := reserveProcessSpool(4); err != nil {
		t.Fatal(err)
	}
	lease := &preparedSpoolLease{path: temporary.Name(), reserved: 4, refs: 1}
	owner := &PreparedDownload{file: temporary, name: temporary.Name(), lease: lease, Size: 4}
	clone, err := clonePreparedDownload(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	processSpoolMu.Lock()
	whileCloneOpen := processSpoolBytes
	processSpoolMu.Unlock()
	if whileCloneOpen != original+4 {
		t.Fatalf("quota released with open clone: %d", whileCloneOpen)
	}
	if _, err := os.Stat(temporary.Name()); err != nil {
		t.Fatalf("spool path removed with open clone: %v", err)
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	processSpoolMu.Lock()
	after := processSpoolBytes
	processSpoolMu.Unlock()
	if after != original {
		t.Fatalf("quota after last close = %d, want %d", after, original)
	}
	if _, err := os.Stat(temporary.Name()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spool remains: %v", err)
	}
}

func TestProcessSpoolQuotaIsReservedBeforeWriting(t *testing.T) {
	root := t.TempDir()
	current := filepath.Join(root, "sshc-sftp-spool-current")
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepareDownloadSpoolDirectory(current); err != nil {
		t.Fatal(err)
	}
	if _, err := reserveDownloadSpool(root, current, 0, 3, 2); !errors.Is(err, ErrTransferLimit) {
		t.Fatalf("over quota reserve = %v", err)
	}
	reserved, err := reserveDownloadSpool(root, current, 0, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if reserved != 2 {
		t.Fatalf("reserved = %d", reserved)
	}
}

func TestCrashSpoolCleanupRejectsSymlinksAndUntrustedDirectories(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.Add(-25 * time.Hour)
	trusted := filepath.Join(root, "sshc-sftp-spool-trusted")
	young := filepath.Join(root, "sshc-sftp-spool-young")
	wide := filepath.Join(root, "sshc-sftp-spool-wide")
	target := filepath.Join(root, "target")
	for _, candidate := range []string{trusted, young, wide, target} {
		if err := os.Mkdir(candidate, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// POSIX mode bits do not restrict a Windows DACL. Build the trusted
	// fixtures through the same platform policy as production so this test
	// verifies cleanup eligibility instead of relying on os.Mkdir's inherited
	// permissions.
	for _, candidate := range []string{trusted, young} {
		if err := prepareDownloadSpoolDirectory(candidate); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(wide, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []string{trusted, wide} {
		if err := os.Chtimes(candidate, old, old); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "sshc-sftp-spool-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	cleanupDownloadSpoolDirectories(root, now)
	if _, err := os.Lstat(trusted); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("trusted stale spool remains: %v", err)
	}
	for _, candidate := range []string{young, wide, link, target} {
		if _, err := os.Lstat(candidate); err != nil {
			t.Fatalf("untrusted candidate %q was removed: %v", candidate, err)
		}
	}
}

func TestCrashSpoolCleanupPreservesLiveOwnerAndRemovesDeadOwnerImmediately(t *testing.T) {
	root := t.TempDir()
	live := filepath.Join(root, "sshc-sftp-spool-live")
	dead := filepath.Join(root, "sshc-sftp-spool-dead")
	for _, candidate := range []string{live, dead} {
		if err := os.Mkdir(candidate, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := prepareDownloadSpoolDirectory(candidate); err != nil {
			t.Fatal(err)
		}
	}
	liveOwner, err := holdDownloadSpoolOwner(live)
	if err != nil {
		t.Fatal(err)
	}
	defer liveOwner.Close()
	deadOwner, err := holdDownloadSpoolOwner(dead)
	if err != nil {
		t.Fatal(err)
	}
	if err := deadOwner.Close(); err != nil {
		t.Fatal(err)
	}

	cleanupDownloadSpoolDirectories(root, time.Now())
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("live process spool removed: %v", err)
	}
	if _, err := os.Stat(dead); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dead process spool remains: %v", err)
	}
}

func TestActiveDownloadSpoolBytesIncludesOtherProcessDirectories(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "sshc-sftp-spool-other")
	current := filepath.Join(root, "sshc-sftp-spool-current")
	for _, candidate := range []string{other, current} {
		if err := os.Mkdir(candidate, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := prepareDownloadSpoolDirectory(candidate); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(other, "archive.part"), []byte("other"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(current, "download.part"), []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := activeDownloadSpoolBytes(root)
	if err != nil {
		t.Fatal(err)
	}
	if got < int64(len("other")+len("current")) {
		t.Fatalf("external spool bytes = %d", got)
	}
}

func TestCrossProcessReservationsShareOneHardQuota(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "sshc-sftp-spool-first")
	second := filepath.Join(root, "sshc-sftp-spool-second")
	for _, candidate := range []string{first, second} {
		if err := os.Mkdir(candidate, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := prepareDownloadSpoolDirectory(candidate); err != nil {
			t.Fatal(err)
		}
	}
	commands := []*exec.Cmd{
		exec.Command(os.Args[0], "-test.run=^TestSpoolQuotaProcessHelper$"),
		exec.Command(os.Args[0], "-test.run=^TestSpoolQuotaProcessHelper$"),
	}
	commands[0].Env = append(os.Environ(), "SSHC_TEST_SPOOL_HELPER=1", "SSHC_TEST_SPOOL_ROOT="+root, "SSHC_TEST_SPOOL_CURRENT="+first)
	commands[1].Env = append(os.Environ(), "SSHC_TEST_SPOOL_HELPER=1", "SSHC_TEST_SPOOL_ROOT="+root, "SSHC_TEST_SPOOL_CURRENT="+second)
	for _, command := range commands {
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	succeeded := 0
	limited := 0
	for _, command := range commands {
		err := command.Wait()
		switch {
		case err == nil:
			succeeded++
		case exitCode(err) == 23:
			limited++
		default:
			t.Fatal(err)
		}
	}
	if succeeded != 1 || limited != 1 {
		t.Fatalf("reservations succeeded=%d limited=%d", succeeded, limited)
	}
	total, err := activeDownloadSpoolBytes(root)
	if err != nil {
		t.Fatal(err)
	}
	if total < 600 || total > 1000 {
		t.Fatalf("global reserved bytes = %d", total)
	}
}

func TestSpoolQuotaProcessHelper(t *testing.T) {
	if os.Getenv("SSHC_TEST_SPOOL_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	_, err := reserveDownloadSpool(
		os.Getenv("SSHC_TEST_SPOOL_ROOT"), os.Getenv("SSHC_TEST_SPOOL_CURRENT"), 0, 600, 1000,
	)
	if err == nil {
		os.Exit(0)
	}
	if errors.Is(err, ErrTransferLimit) {
		os.Exit(23)
	}
	os.Exit(24)
}

func exitCode(err error) int {
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}

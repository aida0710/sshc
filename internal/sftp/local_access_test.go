package sftp

import (
	"errors"
	"io/fs"
	"os"
	"runtime"
	"syscall"
	"testing"
)

func TestOnlyARefusalByTheEnginesOwnSystemIsLabelledLocal(t *testing.T) {
	// pkg/sftp turns the server's SSH_FX_PERMISSION_DENIED into a bare
	// os.ErrPermission, so the server's refusal carries no errno.
	serverRefusal := &fs.PathError{Op: "open", Path: "/srv/report.txt", Err: os.ErrPermission}
	if labelled := labelLocalAccessRefusal(serverRefusal); labelled != error(serverRefusal) {
		t.Fatalf("server refusal = %v, want it unlabelled", labelled)
	}
	missing := &fs.PathError{Op: "open", Path: "/home/me/gone", Err: syscall.ENOENT}
	if labelled := labelLocalAccessRefusal(missing); labelled != error(missing) {
		t.Fatalf("missing file = %v, want it unlabelled", labelled)
	}
	if labelled := labelLocalAccessRefusal(nil); labelled != nil {
		t.Fatalf("no error = %v, want nil", labelled)
	}
	localRefusal := labelLocalAccessRefusal(&fs.PathError{Op: "open", Path: "/home/me/private", Err: syscall.EACCES})
	if !errors.Is(localRefusal, ErrLocalPermissionDenied) || !errors.Is(localRefusal, fs.ErrPermission) {
		t.Fatalf("local refusal = %v, want ErrLocalPermissionDenied that is still a permission error", localRefusal)
	}
}

func TestMacOSPrivacyProtectionIsToldApartFromFilePermissions(t *testing.T) {
	// macOS answers a folder such as Downloads that the user has not allowed
	// with EPERM. Elsewhere EPERM is an ordinary permission refusal.
	refusal := labelLocalAccessRefusal(&fs.PathError{Op: "open", Path: "/Users/me/Downloads", Err: syscall.EPERM})
	want, other := ErrLocalPermissionDenied, ErrLocalPrivacyProtection
	if runtime.GOOS == "darwin" {
		want, other = ErrLocalPrivacyProtection, ErrLocalPermissionDenied
	}
	if !errors.Is(refusal, want) || errors.Is(refusal, other) {
		t.Fatalf("EPERM on %s = %v, want only %v", runtime.GOOS, refusal, want)
	}
}

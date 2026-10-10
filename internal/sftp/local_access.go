package sftp

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
)

var (
	// ErrLocalPermissionDenied is the operating system of the engine's machine
	// refusing a file operation on the local side. The screen tells it apart
	// from the SFTP server refusing one, whose permissions live elsewhere.
	ErrLocalPermissionDenied = errors.New("the engine's operating system denied access to a local file")
	// ErrLocalPrivacyProtection is macOS's privacy protection keeping the
	// engine out of a folder such as Downloads, Documents or Desktop. File
	// permissions do not lift it; only the user can, in System Settings.
	ErrLocalPrivacyProtection = errors.New("macOS privacy protection denied the engine access to a local file")
)

// labelLocalAccessRefusal adds ErrLocalPermissionDenied or
// ErrLocalPrivacyProtection to err when the operating system of the engine's
// machine refused it. The SFTP server's refusal reaches the engine as
// pkg/sftp's os.ErrPermission, which carries no errno, so an errno in the
// chain always comes from this machine.
func labelLocalAccessRefusal(err error) error {
	if errors.Is(err, ErrLocalPermissionDenied) || errors.Is(err, ErrLocalPrivacyProtection) {
		return err
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) || !errors.Is(errno, fs.ErrPermission) {
		return err
	}
	if isPrivacyProtectionRefusal(errno) {
		return fmt.Errorf("%w: %w", ErrLocalPrivacyProtection, err)
	}
	return fmt.Errorf("%w: %w", ErrLocalPermissionDenied, err)
}

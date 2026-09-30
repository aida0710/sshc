package sftp_test

import (
	"testing"

	"sshc/internal/sftp"
)

// newTestTransferManager builds a manager whose download spool lives in the
// test's temporary directory. It closes the manager before that directory is
// removed, because Windows cannot remove a spool whose owner lock is open.
func newTestTransferManager(t testing.TB, service *sftp.Service) *sftp.TransferManager {
	t.Helper()
	manager := sftp.NewTransferManager(service, t.TempDir())
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}

//go:build android

package serialtransport

import "io"

// Android applications cannot open USB serial adapters through Linux /dev
// paths. Supporting them requires Android USB Host permission and a native
// USB-serial driver, so the desktop backend is intentionally unavailable.
func (systemBackend) ListPorts() ([]string, error) { return nil, ErrPlatformUnsupported }

func (systemBackend) OpenPort(string, Mode) (io.ReadWriteCloser, error) {
	return nil, ErrPlatformUnsupported
}

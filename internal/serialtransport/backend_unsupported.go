//go:build !darwin && !linux && !windows

package serialtransport

import "io"

func (systemBackend) ListPorts() ([]string, error) { return nil, ErrPlatformUnsupported }

func (systemBackend) OpenPort(string, Mode) (io.ReadWriteCloser, error) {
	return nil, ErrPlatformUnsupported
}

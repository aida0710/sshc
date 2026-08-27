package serialtransport

import (
	"errors"
	"fmt"

	serial "go.bug.st/serial"
)

type systemBackend struct{}

var _ Backend = systemBackend{}

type portErrorCoder interface{ Code() serial.PortErrorCode }

func classifyBackendError(operation, device string, err error) error {
	if err == nil {
		return nil
	}
	classified := err
	var portError portErrorCoder
	if errors.As(err, &portError) {
		switch portError.Code() {
		case serial.PortBusy:
			classified = errors.Join(ErrPortBusy, err)
		case serial.PortNotFound:
			classified = errors.Join(ErrPortNotFound, err)
		case serial.PermissionDenied:
			classified = errors.Join(ErrPermissionDenied, err)
		case serial.InvalidSerialPort:
			classified = errors.Join(ErrInvalidPort, err)
		case serial.ErrorEnumeratingPorts:
			classified = errors.Join(ErrEnumeration, err)
		case serial.PortClosed:
			classified = errors.Join(ErrClosed, err)
		}
	}
	if device == "" {
		return fmt.Errorf("%s: %w", operation, classified)
	}
	return fmt.Errorf("%s %q: %w", operation, device, classified)
}

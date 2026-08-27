//go:build (linux || windows || (darwin && cgo)) && !android

package serialtransport

import (
	"errors"

	serial "go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

// ListDevices uses native USB descriptors where the enumerator backend is
// available. The Darwin implementation uses IOKit through cgo, so !cgo builds
// intentionally fall back to the plain ListPorts method on systemBackend.
func (systemBackend) ListDevices() ([]Device, error) {
	details, err := enumerator.GetDetailedPortsList()
	if err == nil {
		devices := make([]Device, 0, len(details))
		for _, detail := range details {
			devices = append(devices, Device{
				Name: detail.Name, USB: detail.IsUSB, VID: detail.VID, PID: detail.PID,
				SerialNumber: detail.SerialNumber, Product: detail.Product, Manufacturer: detail.Manufacturer,
			})
		}
		return devices, nil
	}
	var unsupported *enumerator.PortEnumerationError
	if !errors.As(err, &unsupported) {
		return nil, err
	}
	names, fallbackErr := serial.GetPortsList()
	if fallbackErr != nil {
		return nil, fallbackErr
	}
	devices := make([]Device, len(names))
	for index, name := range names {
		devices[index] = Device{Name: name}
	}
	return devices, nil
}

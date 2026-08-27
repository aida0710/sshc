//go:build (darwin || linux || windows) && !android

package serialtransport

import (
	"io"

	serial "go.bug.st/serial"
)

func (systemBackend) ListPorts() ([]string, error) { return serial.GetPortsList() }

func (systemBackend) OpenPort(device string, mode Mode) (io.ReadWriteCloser, error) {
	if mode.FlowControl != FlowControlNone {
		return nil, ErrUnsupportedFlowControl
	}
	port, err := serial.Open(device, &serial.Mode{
		BaudRate: mode.BaudRate,
		DataBits: mode.DataBits,
		Parity:   nativeParity(mode.Parity),
		StopBits: nativeStopBits(mode.StopBits),
	})
	if err != nil {
		return nil, err
	}
	return port, nil
}

func nativeParity(parity Parity) serial.Parity {
	switch parity {
	case ParityOdd:
		return serial.OddParity
	case ParityEven:
		return serial.EvenParity
	case ParityMark:
		return serial.MarkParity
	case ParitySpace:
		return serial.SpaceParity
	default:
		return serial.NoParity
	}
}

func nativeStopBits(stopBits StopBits) serial.StopBits {
	if stopBits == StopBitsOnePointFive {
		return serial.OnePointFiveStopBits
	}
	if stopBits == StopBitsTwo {
		return serial.TwoStopBits
	}
	return serial.OneStopBit
}

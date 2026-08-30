//go:build (darwin || linux || windows) && !android

package serialtransport

import (
	"io"

	serial "go.bug.st/serial"
	driver "sshc/third_party/go-serial"
)

func (systemBackend) ListPorts() ([]string, error) { return serial.GetPortsList() }

func (systemBackend) OpenPort(device string, mode Mode) (io.ReadWriteCloser, error) {
	port, err := driver.Open(device, &driver.Mode{
		BaudRate:    mode.BaudRate,
		DataBits:    mode.DataBits,
		Parity:      nativeParity(mode.Parity),
		StopBits:    nativeStopBits(mode.StopBits),
		FlowControl: nativeFlowControl(mode.FlowControl),
	})
	if err != nil {
		return nil, err
	}
	return port, nil
}

func nativeFlowControl(flow FlowControl) driver.FlowControl {
	switch flow {
	case FlowControlRTSCTS:
		return driver.RTSCTSFlowControl
	case FlowControlXONXOFF:
		return driver.XONXOFFFlowControl
	default:
		return driver.NoFlowControl
	}
}

func nativeParity(parity Parity) driver.Parity {
	switch parity {
	case ParityOdd:
		return driver.OddParity
	case ParityEven:
		return driver.EvenParity
	case ParityMark:
		return driver.MarkParity
	case ParitySpace:
		return driver.SpaceParity
	default:
		return driver.NoParity
	}
}

func nativeStopBits(stopBits StopBits) driver.StopBits {
	if stopBits == StopBitsOnePointFive {
		return driver.OnePointFiveStopBits
	}
	if stopBits == StopBitsTwo {
		return driver.TwoStopBits
	}
	return driver.OneStopBit
}

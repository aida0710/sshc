//go:build linux

package serialtransport

import (
	"context"
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestSystemBackendAppliesFlowControl(t *testing.T) {
	tests := []struct {
		name        string
		flow        FlowControl
		wantRTSCTS  bool
		wantXONXOFF bool
	}{
		{name: "none", flow: FlowControlNone},
		{name: "rtscts", flow: FlowControlRTSCTS, wantRTSCTS: true},
		{name: "xonxoff", flow: FlowControlXONXOFF, wantXONXOFF: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			master, slave, err := pty.Open()
			if err != nil {
				t.Skipf("open PTY: %v", err)
			}
			defer master.Close()
			defer slave.Close()

			config := DefaultConfig(slave.Name())
			config.FlowControl = test.flow
			stream, err := New().Open(context.Background(), config)
			if err != nil {
				t.Fatalf("Open(%s): %v", test.flow, err)
			}
			defer stream.Close()

			settings, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
			if err != nil {
				t.Fatalf("read PTY termios: %v", err)
			}
			if got := settings.Cflag&unix.CRTSCTS != 0; got != test.wantRTSCTS {
				t.Errorf("CRTSCTS enabled = %t, want %t", got, test.wantRTSCTS)
			}
			xonxoff := settings.Iflag&unix.IXON != 0 && settings.Iflag&unix.IXOFF != 0
			if xonxoff != test.wantXONXOFF {
				t.Errorf("IXON and IXOFF enabled = %t, want %t", xonxoff, test.wantXONXOFF)
			}
			if test.wantXONXOFF && settings.Iflag&unix.IXANY != 0 {
				t.Error("IXANY is enabled with bidirectional XON/XOFF")
			}
		})
	}
}

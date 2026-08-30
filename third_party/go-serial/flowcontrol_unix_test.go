//go:build linux || darwin || freebsd || openbsd

package serial

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestSetTermSettingsFlowControl(t *testing.T) {
	tests := []struct {
		name        string
		flow        FlowControl
		wantRTSCTS  bool
		wantXONXOFF bool
	}{
		{name: "none", flow: NoFlowControl},
		{name: "rtscts", flow: RTSCTSFlowControl, wantRTSCTS: true},
		{name: "xonxoff", flow: XONXOFFFlowControl, wantXONXOFF: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := &unix.Termios{
				Cflag: tcCRTSCTS,
				Iflag: unix.IXON | unix.IXOFF | unix.IXANY,
			}
			setTermSettingsFlowControl(test.flow, settings)
			if got := settings.Cflag&tcCRTSCTS != 0; got != test.wantRTSCTS {
				t.Errorf("RTS/CTS enabled = %t, want %t", got, test.wantRTSCTS)
			}
			xonxoff := settings.Iflag&unix.IXON != 0 && settings.Iflag&unix.IXOFF != 0
			if xonxoff != test.wantXONXOFF {
				t.Errorf("XON/XOFF enabled = %t, want %t", xonxoff, test.wantXONXOFF)
			}
			if settings.Iflag&unix.IXANY != 0 {
				t.Error("IXANY remains enabled")
			}
		})
	}
}

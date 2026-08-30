//go:build windows

package serial

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestSetModeFlowControl(t *testing.T) {
	tests := []struct {
		name      string
		flow      FlowControl
		initial   uint32
		wantFlags uint32
	}{
		{name: "none preserves disabled RTS", flow: NoFlowControl},
		{name: "none leaves handshake in manual mode", flow: NoFlowControl, initial: dcbRTSControlHandshake, wantFlags: dcbRTSControlEnable},
		{name: "rtscts", flow: RTSCTSFlowControl, initial: dcbInX | dcbOutX, wantFlags: dcbOutXCTSFlow | dcbRTSControlHandshake},
		{name: "xonxoff", flow: XONXOFFFlowControl, initial: dcbRTSControlEnable | dcbOutXCTSFlow, wantFlags: dcbRTSControlEnable | dcbInX | dcbOutX},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			settings := &windows.DCB{Flags: test.initial}
			setModeFlowControl(test.flow, settings)
			const relevant = dcbRTSControlMask | dcbOutXCTSFlow | dcbInX | dcbOutX
			if got := settings.Flags & relevant; got != test.wantFlags {
				t.Errorf("flow flags = %#x, want %#x", got, test.wantFlags)
			}
			if settings.XonChar != 17 || settings.XoffChar != 19 {
				t.Errorf("XON/XOFF characters = %d/%d", settings.XonChar, settings.XoffChar)
			}
		})
	}
}

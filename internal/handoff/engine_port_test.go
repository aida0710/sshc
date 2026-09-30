package handoff_test

import (
	"errors"
	"testing"

	"sshc/internal/handoff"
)

func TestEnginePortRefusesPrivilegedAndOutOfRangeNumbers(t *testing.T) {
	for _, port := range []int{1024, 43123, 65535} {
		if err := handoff.EnginePort(port); err != nil {
			t.Errorf("EnginePort(%d) = %v, want nil", port, err)
		}
	}
	for _, port := range []int{0, 22, 1023, 65536} {
		if err := handoff.EnginePort(port); !errors.Is(err, handoff.ErrEnginePort) {
			t.Errorf("EnginePort(%d) = %v, want ErrEnginePort", port, err)
		}
	}
}

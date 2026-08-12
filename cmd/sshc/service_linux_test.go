//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sshc/internal/platform"
	"sshc/internal/platform/linux"
)

type linuxServiceRunner struct{}

func (linuxServiceRunner) RunOutput(_ context.Context, _ platform.Command) (platform.Output, error) {
	return platform.Output{}, nil
}

func systemctlStat(result error) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if path != linux.DefaultSystemctl {
			return nil, errors.New("unexpected stat path: " + path)
		}
		return nil, result
	}
}

func TestLinuxServiceControllerIsAvailableWhenSystemctlExists(t *testing.T) {
	item, err := newLinuxServiceLoginItem(t.TempDir(), systemctlStat(nil), linuxServiceRunner{})
	if err != nil || item == nil {
		t.Fatalf("controller = %T, %v; want non-nil, nil", item, err)
	}
}

func TestLinuxServiceControllerIsANoopWhenSystemctlAndUnitAreAbsent(t *testing.T) {
	item, err := newLinuxServiceLoginItem(t.TempDir(), systemctlStat(os.ErrNotExist), linuxServiceRunner{})
	if err != nil || item != nil {
		t.Fatalf("controller = %T, %v; want nil, nil", item, err)
	}
}

func TestLinuxServiceControllerRejectsAStrandedUnitWithoutSystemctl(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".config", "systemd", "user", linux.UnitName)
	if err := os.MkdirAll(filepath.Dir(unit), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unit, []byte("[Service]\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	item, err := newLinuxServiceLoginItem(home, systemctlStat(os.ErrNotExist), linuxServiceRunner{})
	if err == nil || item != nil {
		t.Fatalf("controller = %T, %v; want nil and an error", item, err)
	}
}

func TestLinuxServiceControllerReportsUnknownSystemctlAndUnitState(t *testing.T) {
	t.Run("systemctl", func(t *testing.T) {
		item, err := newLinuxServiceLoginItem(t.TempDir(), systemctlStat(os.ErrPermission), linuxServiceRunner{})
		if err == nil || item != nil {
			t.Fatalf("controller = %T, %v; want nil and an error", item, err)
		}
	})

	t.Run("unit", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, ".config"), []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, systemctlResult := range []error{nil, os.ErrNotExist} {
			item, err := newLinuxServiceLoginItem(home, systemctlStat(systemctlResult), linuxServiceRunner{})
			if err == nil || item != nil {
				t.Errorf("systemctl error %v: controller = %T, %v; want nil and an error",
					systemctlResult, item, err)
			}
		}
	})
}

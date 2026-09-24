package vpn

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// docker は、起動する環境の PATH で探す。engine 自身の PATH では探さない。
//
// launchd が起動した engine の PATH には、Docker Desktop の /usr/local/bin が無い。
func TestDockerIsFoundInThePathItWillRunWith(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("実行の許可の bit で確かめる")
	}
	first, second := t.TempDir(), t.TempDir()
	docker := filepath.Join(second, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// 実行できないファイルは飛ばす。
	if err := os.WriteFile(filepath.Join(first, "docker"), []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	variables := []string{"PATH=/nowhere", "HOME=/home/user", "PATH=relative" + string(filepath.ListSeparator) +
		first + string(filepath.ListSeparator) + second}

	found, err := lookPathIn("docker", pathVariable(variables))
	if err != nil || found != docker {
		t.Fatalf("lookPathIn = %q, %v", found, err)
	}
	if _, err := lookPathIn("docker", "/nowhere"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("lookPathIn(無い) = %v", err)
	}
}

// トンネルのデバイスを渡せなかった docker run は、デバイスが無いことを理由にする。
func TestARunThatCouldNotPassTheDeviceSaysSo(t *testing.T) {
	refused := errors.New(`docker: Error response from daemon: error gathering device information while adding custom device "/dev/ppp": no such file or directory`)

	if err := runFailure("/dev/ppp", refused); !errors.Is(err, ErrTunnelDevice) {
		t.Fatalf("runFailure = %v", err)
	}
	if err := runFailure("/dev/ppp", errors.New("conflict")); !errors.Is(err, ErrSessionFailed) {
		t.Fatalf("runFailure = %v", err)
	}
}

// 見せるログは上限の中に収め、文字の途中で切らない。
func TestShownLogsKeepTheirNewestPartWithinTheLimit(t *testing.T) {
	text := "古い行\n新しい行"

	if got := lastBytes(text, 100); got != text {
		t.Fatalf("lastBytes = %q", got)
	}
	// 上限が「古い行」の「行」の途中に当たっても、文字の頭から始める。
	got := lastBytes(text, len("\n新しい行")+1)
	if got != "\n新しい行" {
		t.Fatalf("lastBytes = %q", got)
	}
}

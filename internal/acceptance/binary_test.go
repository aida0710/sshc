package acceptance_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 旧版の launchd / systemd unit が使った -open=false を受け付けると、unit が
// 新しいバイナリを起こした時点で engine.lock を先に握り、デスクトップの子が
// 上がれなくなる。旧版からの移行を持たない以上、黙って常駐するより未定義の
// フラグとして直ちに拒む。
func TestLegacyOpenFlagIsRejected(t *testing.T) {
	repository := filepath.Join("..", "..")
	binary := filepath.Join(t.TempDir(), builtBinaryName)
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/sshc")
	build.Dir = repository
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build = %v\n%s", err, output)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	process := exec.CommandContext(ctx, binary, "-open=false")
	process.Env = isolatedEnvironment(t.TempDir())
	output, err := process.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("-open=false did not exit immediately: %v", ctx.Err())
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("-open=false exit = %v; want status 2\n%s", err, output)
	}
	if !strings.Contains(string(output), `unknown command "-open=false"`) {
		t.Fatalf("-open=false output did not explain the rejected flag:\n%s", output)
	}
}

// TestNoTestOnlyPackageReachesTheShippedBinary は、hardening
// suite を artefact の外に保つ。internal/acceptance は構造上
// test-only だが、将来ヘルパーが非テストファイルへ移されれば、それは黙って崩れる。
func TestNoTestOnlyPackageReachesTheShippedBinary(t *testing.T) {
	list := exec.Command("go", "list", "-deps", "./cmd/sshc")
	list.Dir = filepath.Join("..", "..")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("go list = %v\n%s", err, output)
	}
	seen := 0
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			seen++
		}
		switch trimmed {
		case "sshc/internal/acceptance":
			t.Error("the hardening suite is linked into the shipped binary")
		case "testing", "net/http/httptest":
			t.Errorf("%s is linked into the shipped binary", trimmed)
		}
	}
	if seen == 0 {
		t.Fatal("go list reported no dependency at all; this check is not looking at the binary")
	}
}

// TestNoTestOnlyPackageReachesTheAndroidLibrary は、AAR についても同じことを言う。
// 出荷物が 2 つになったので、規則も 2 つに対して立てる。
func TestNoTestOnlyPackageReachesTheAndroidLibrary(t *testing.T) {
	list := exec.Command("go", "list", "-deps", "./mobile")
	list.Dir = filepath.Join("..", "..")
	output, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("go list = %v\n%s", err, output)
	}
	seen := 0
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			seen++
		}
		switch trimmed {
		case "sshc/internal/acceptance":
			t.Error("the hardening suite is linked into the Android library")
		case "testing", "net/http/httptest":
			t.Errorf("%s is linked into the Android library", trimmed)
		}
	}
	if seen == 0 {
		t.Fatal("go list reported no dependency at all; this check is not looking at the library")
	}
}

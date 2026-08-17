//go:build windows

package windowsregistry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// testKey は、このテストひとつぶんのレジストリの枝を作る。
//
// **production の枝を使わない。** テストを走らせた人の機械に、そのアプリの
// 起動登録を書き換えた痕跡を残してはならない。
func testKey(t *testing.T) string {
	t.Helper()
	key := fmt.Sprintf(`Software\sshc\test-%d-%s`, os.Getpid(), strings.ReplaceAll(t.Name(), "/", "-"))
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, key)
	})
	return key
}

// desktopExecutable は、起こせる形をした実体をひとつ置く。中身は問わない
// ——確かめているのは、この値を起こしてよいかの判断だけである。
func desktopExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), desktopExecutableName)
	if err := os.WriteFile(path, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// **登録が無いことは、直し方のある状態である。** 何も言わずに黙ると、
// `sshc` と打った人には理由の無い失敗だけが残る。
func TestAnUnregisteredDesktopSaysHowToRegisterOne(t *testing.T) {
	key := testKey(t)

	_, err := readDesktopExecutable(key)

	if !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("err = %v, want ErrNotRegistered", err)
	}
	if !strings.Contains(err.Error(), "installer") {
		t.Errorf("err = %q, want the installer named", err)
	}
}

// **記録された文字列を、そのまま実行に渡さない。** ここに書ける相手はこの
// 利用者に限られるが、書き換えられたことに気づく手立ては要る。
func TestOnlyAnAbsoluteRegularFileNamedSshcIsAccepted(t *testing.T) {
	directory := t.TempDir()
	wrongName := filepath.Join(directory, "notepad.exe")
	if err := os.WriteFile(wrongName, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"empty":            "",
		"relative":         `sshc.exe`,
		"dot relative":     `.\sshc.exe`,
		"a directory":      directory,
		"a missing file":   filepath.Join(directory, "absent", desktopExecutableName),
		"another program":  wrongName,
		"a device path":    `\\.\pipe\sshc.exe`,
		"an extended path": `\\?\C:\Program Files\sshc\sshc.exe`,
	} {
		key := testKey(t)
		if err := registerDesktopExecutable(key, path); !errors.Is(err, ErrUnusableExecutable) {
			t.Errorf("registering %s (%q) = %v, want ErrUnusableExecutable", name, path, err)
		}
	}

	key := testKey(t)
	accepted := desktopExecutable(t)
	if err := registerDesktopExecutable(key, accepted); err != nil {
		t.Fatalf("an absolute sshc.exe was refused: %v", err)
	}
	stored, err := readDesktopExecutable(key)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if stored != accepted {
		t.Errorf("stored = %q, want %q", stored, accepted)
	}
}

// 書いたあとに実体が消えることはある。**そのときも、そのまま起こさない。**
func TestARegisteredExecutableThatIsGoneIsRefusedWithItsPath(t *testing.T) {
	key := testKey(t)
	path := desktopExecutable(t)
	if err := registerDesktopExecutable(key, path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	_, err := readDesktopExecutable(key)

	if !errors.Is(err, ErrUnusableExecutable) {
		t.Fatalf("err = %v, want ErrUnusableExecutable", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %q, want the recorded path named", err)
	}
}

// **他人の記録を消さない。** 二つの版が入っている機械では、アンインストーラは
// 自分が書いたものだけを消す必要がある。
func TestRemoveOnlyDeletesTheValueItWasToldToExpect(t *testing.T) {
	key := testKey(t)
	mine := desktopExecutable(t)
	if err := registerDesktopExecutable(key, mine); err != nil {
		t.Fatal(err)
	}

	if err := removeDesktopExecutable(key, filepath.Join(t.TempDir(), desktopExecutableName)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if stored, err := readDesktopExecutable(key); err != nil || stored != mine {
		t.Fatalf("another installation's value was removed: %q, %v", stored, err)
	}

	if err := removeDesktopExecutable(key, mine); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := readDesktopExecutable(key); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("after removing its own value the key still reads %v", err)
	}
}

// Windows のパスは大文字小文字を区別しない。同じ実体を指す二つの綴りを別物と
// 扱うと、**正しいアンインストーラが自分の記録を消せなくなる。**
func TestRemoveMatchesTheStoredValueCaseInsensitively(t *testing.T) {
	key := testKey(t)
	path := desktopExecutable(t)
	if err := registerDesktopExecutable(key, path); err != nil {
		t.Fatal(err)
	}

	if err := removeDesktopExecutable(key, strings.ToUpper(path)); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := readDesktopExecutable(key); !errors.Is(err, ErrNotRegistered) {
		t.Fatalf("a differently-cased spelling of the same file did not match: %v", err)
	}
}

// 消すものが無いことは失敗ではない。アンインストーラは、登録される前に
// 中断されたインストールのあとにも走る。
func TestRemovingAnAbsentRegistrationIsNotAnError(t *testing.T) {
	if err := removeDesktopExecutable(testKey(t), `C:\nowhere\sshc.exe`); err != nil {
		t.Errorf("remove without a registration = %v, want nil", err)
	}
}

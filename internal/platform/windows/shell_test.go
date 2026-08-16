package windows_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sshc/internal/platform/windows"
)

// lookupOf は、この検査が渡す環境そのものである。ここに無い名前は「置かれて
// いない」を意味する。
func lookupOf(environment map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := environment[name]
		return value, ok
	}
}

// existing は、その綴りのファイルだけが実在するファイルシステムを演じる。
//
// 実ファイルを置かずに済ませるのは、ここで確かめたいのがドライブ文字を持つ
// Windows の綴りだからである。macOS の一時ディレクトリでは、拒むべき綴りと
// 受け入れるべき綴りの区別そのものが作れない。
func existing(paths ...string) func(string) error {
	return func(path string) error {
		if slices.Contains(paths, path) {
			return nil
		}
		return os.ErrNotExist
	}
}

func writeShell(t *testing.T, elements ...string) string {
	t.Helper()
	path := filepath.Join(elements...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 順番は上から下である。PowerShell 7 が入っているマシンで 5.1 を開くのは、
// 利用者が今使っているシェルではない。
func TestTheLoginShellPrefersPowerShellSeven(t *testing.T) {
	programFiles, windowsDirectory := t.TempDir(), t.TempDir()
	want := writeShell(t, programFiles, "PowerShell", "7", "pwsh.exe")
	writeShell(t, windowsDirectory, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")

	shell, err := windows.LoginShell(lookupOf(map[string]string{
		"ProgramFiles": programFiles, "WINDIR": windowsDirectory,
	}), nil)
	if err != nil {
		t.Fatalf("LoginShell() = %v", err)
	}
	if shell != want {
		t.Errorf("LoginShell() = %q, want %q", shell, want)
	}
}

// 同梱の Windows PowerShell は、どの Windows にもある。PowerShell 7 が入って
// いないマシンで端末が開かないことは、あってはならない。
func TestTheLoginShellFallsBackToTheBundledWindowsPowerShell(t *testing.T) {
	programFiles, windowsDirectory := t.TempDir(), t.TempDir()
	want := writeShell(t, windowsDirectory, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")

	shell, err := windows.LoginShell(lookupOf(map[string]string{
		"ProgramFiles": programFiles, "WINDIR": windowsDirectory,
	}), nil)
	if err != nil {
		t.Fatalf("LoginShell() = %v", err)
	}
	if shell != want {
		t.Errorf("LoginShell() = %q, want %q", shell, want)
	}
}

func TestTheLoginShellFallsBackToComSpec(t *testing.T) {
	comSpec := `C:\Windows\System32\cmd.exe`

	shell, err := windows.LoginShell(
		lookupOf(map[string]string{"ComSpec": comSpec}), existing(comSpec),
	)
	if err != nil {
		t.Fatalf("LoginShell() = %v", err)
	}
	if shell != comSpec {
		t.Errorf("LoginShell() = %q, want %q", shell, comSpec)
	}
}

// **PATH に置かれた pwsh.exe は決して勝たない。**
//
// 端末に渡すのは利用者のシェルであって、環境がたまたま先に挙げたプログラムでは
// ない。信頼できる場所に一本も無いなら、開かないことが答えである。
func TestTheLoginShellNeverTakesAShellFromThePath(t *testing.T) {
	poisoned := t.TempDir()
	writeShell(t, poisoned, "pwsh.exe")
	writeShell(t, poisoned, "powershell.exe")
	writeShell(t, poisoned, "cmd.exe")
	t.Setenv("PATH", poisoned)

	// SHELL も同じである。Windows でそれを置くのは MSYS や Cygwin であり、
	// 利用者が選んだのではなく、何かを入れた副作用である。
	shell, err := windows.LoginShell(lookupOf(map[string]string{
		"PATH":  poisoned,
		"SHELL": filepath.Join(poisoned, "pwsh.exe"),
	}), nil)
	if !errors.Is(err, windows.ErrNoLoginShell) {
		t.Fatalf("LoginShell() = %q, %v; want ErrNoLoginShell", shell, err)
	}
}

func TestTheLoginShellFindsNothingInAnEmptyEnvironment(t *testing.T) {
	for name, lookup := range map[string]func(string) (string, bool){
		"nil":   nil,
		"empty": lookupOf(nil),
		"blank": lookupOf(map[string]string{"ProgramFiles": "", "WINDIR": "", "ComSpec": ""}),
	} {
		t.Run(name, func(t *testing.T) {
			if shell, err := windows.LoginShell(lookup, existing()); !errors.Is(err, windows.ErrNoLoginShell) {
				t.Fatalf("LoginShell() = %q, %v; want ErrNoLoginShell", shell, err)
			}
		})
	}
}

// **%ComSpec% だけは利用者の環境から来る。**
//
// だから、そこに書かれた綴りは実行してよいプログラムの形をしていなければ
// ならない。ここに並ぶのはどれも「実在する」と答えるファイルシステムの上で
// 拒まれる——拒む理由は綴りそのものにあり、そこに何があるかではない。
func TestTheLoginShellRefusesAnUntrustedComSpec(t *testing.T) {
	for name, value := range map[string]string{
		"relative":          `cmd.exe`,
		"drive relative":    `\Windows\System32\cmd.exe`,
		"quoted":            `"C:\Windows\System32\cmd.exe"`,
		"argument bearing":  `C:\Windows\System32\cmd.exe /c whoami`,
		"extended":          `\\?\C:\Windows\System32\cmd.exe`,
		"device":            `\\.\C:\Windows\System32\cmd.exe`,
		"nt object":         `\??\C:\Windows\System32\cmd.exe`,
		"unc share":         `\\attacker\share\cmd.exe`,
		"parent traversal":  `C:\Windows\..\Users\someone\cmd.exe`,
		"alternate stream":  `C:\Windows\System32\cmd.exe:evil`,
		"not a program":     `C:\Windows\System32\shell.bat`,
		"bare directory":    `C:\Windows\System32`,
		"empty":             ``,
		"embedded newline":  "C:\\Windows\\System32\\cmd.exe\nwhoami",
		"embedded nul byte": "C:\\Windows\\System32\\cmd.exe\x00",
	} {
		t.Run(name, func(t *testing.T) {
			shell, err := windows.LoginShell(
				lookupOf(map[string]string{"ComSpec": value}),
				func(string) error { return nil },
			)
			if !errors.Is(err, windows.ErrNoLoginShell) {
				t.Fatalf("LoginShell(%q) = %q, %v; want ErrNoLoginShell", value, shell, err)
			}
		})
	}
}

// 空白も Unicode も、ドライブ文字の大小も、Windows のパスには普通にある。
func TestTheLoginShellAcceptsOrdinaryWindowsSpellings(t *testing.T) {
	for name, value := range map[string]string{
		"spaces":          `C:\Program Files\Ops Tools\cmd.exe`,
		"unicode":         `C:\ユーザー\道具\cmd.exe`,
		"lowercase drive": `c:\windows\system32\cmd.exe`,
		"forward slashes": `C:/Windows/System32/cmd.exe`,
		"mixed case name": `C:\Windows\System32\CMD.EXE`,
	} {
		t.Run(name, func(t *testing.T) {
			shell, err := windows.LoginShell(
				lookupOf(map[string]string{"ComSpec": value}), existing(value),
			)
			if err != nil {
				t.Fatalf("LoginShell(%q) = %v", value, err)
			}
			if shell != value {
				t.Errorf("LoginShell() = %q, want %q", shell, value)
			}
		})
	}
}

// **PowerShell は起動のたびに著作権表示を出す。** 端末を開いた利用者が最初に
// 見るのはそれではない。プロファイルは読ませる——開いているのは利用者のシェルで
// あって、素のインタプリタではないからだ。
func TestLoginArgumentsSilenceOnlyTheBanner(t *testing.T) {
	for name, expected := range map[string]struct {
		shell string
		want  []string
	}{
		"powershell 7":       {`C:\Program Files\PowerShell\7\pwsh.exe`, []string{"-NoLogo"}},
		"windows powershell": {`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, []string{"-NoLogo"}},
		"mixed case":         {`C:\Windows\PWSH.EXE`, []string{"-NoLogo"}},
		"command processor":  {`C:\Windows\System32\cmd.exe`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := windows.LoginArguments(expected.shell); !slices.Equal(got, expected.want) {
				t.Errorf("LoginArguments(%q) = %q, want %q", expected.shell, got, expected.want)
			}
		})
	}
}

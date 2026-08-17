//go:build !windows

package windowsregistry

import "errors"

// LauncherKey と LauncherValue は、Windows のレジストリの場所である。
// **他所では場所ではない。** ここに置いてあるのは、この綴りを読む側が
// build tag を持たずに済むためであり、使えることを意味しない。
const (
	LauncherKey   = `Software\sshc\Desktop`
	LauncherValue = "Executable"
)

var (
	// ErrNotRegistered と ErrUnusableExecutable は、Windows 側と対の名前である。
	ErrNotRegistered      = errors.New("no sshc desktop application is registered for this user")
	ErrUnusableExecutable = errors.New("the registered sshc desktop application cannot be started")
)

// errNoRegistry は、この OS にレジストリが無いことを述べる。
//
// **代わりのものを推測しない。** ここに設定ファイルへの経路を置くと、
// 「レジストリを読んだ」と言いながら別のものを読む関数になる。
var errNoRegistry = errors.New("the Windows registry exists only on Windows")

func ReadDesktopExecutable() (string, error) { return "", errNoRegistry }

func RegisterDesktopExecutable(string) error { return errNoRegistry }

func RemoveDesktopExecutable(string) error { return errNoRegistry }

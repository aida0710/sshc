//go:build windows

package application

import (
	"errors"
	"testing"
)

const testHome = `C:\Users\Tester`

const testOutside = `D:\shared\ssh_config`

// 要素に `C:` を含むパスは、filepath.Rel が `C:\x` という絶対パスを返す。それを
// ~/.ssh の中と答えると、下の storage が外として拒む操作を application だけが通す。
func TestWindowsPathsWithADriveLikeComponentAreOutsideTheRoot(t *testing.T) {
	root := testRoot
	if relative, err := RelativePath(root, root+`\C:\x`); !errors.Is(err, ErrExternalPath) {
		t.Errorf("RelativePath = %q, %v; want ErrExternalPath", relative, err)
	}
	if absolute, err := AbsolutePath(root, "C:/x"); !errors.Is(err, ErrExternalPath) {
		t.Errorf("AbsolutePath = %q, %v; want ErrExternalPath", absolute, err)
	}
}

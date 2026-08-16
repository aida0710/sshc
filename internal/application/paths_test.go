package application

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// uncleaned は、掃除される前の綴りを組み立てる。filepath.Join は Clean を通すので、
// "掃除したら何になるか" を尋ねる検査は、Join で作った時点で消えてしまう。
func uncleaned(elements ...string) string {
	return strings.Join(elements, string(filepath.Separator))
}

func TestRelativePathRejectsEverythingOutsideTheRoot(t *testing.T) {
	root := testRoot
	// 戻り値はスラッシュ区切りの識別子である。**入力だけがネイティブなパスで
	// あって、答えはどの OS でも同じ綴りでなければならない。** UI と metadata が
	// 持ち回るのはこの識別子だからだ。
	tests := []struct {
		name     string
		absolute string
		want     string
		wantErr  bool
	}{
		{"root child", filepath.Join(root, "config"), "config", false},
		{"nested child", filepath.Join(root, "conf.d", "10-home.conf"), "conf.d/10-home.conf", false},
		{"uncleaned child", uncleaned(root, "conf.d", "..", "config"), "config", false},
		{"the root itself", root, "", true},
		{"sibling directory", filepath.Join(testHome, ".sshother", "config"), "", true},
		{"escaping parent", uncleaned(root, "..", ".bashrc"), "", true},
		{"unrelated absolute", testOutside, "", true},
		{"relative input", "conf.d/10-home.conf", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			relative, err := RelativePath(root, test.absolute)
			if test.wantErr {
				if !errors.Is(err, ErrExternalPath) {
					t.Fatalf("RelativePath(%q) = %q, %v; want ErrExternalPath", test.absolute, relative, err)
				}
				return
			}
			if err != nil || relative != test.want {
				t.Fatalf("RelativePath(%q) = %q, %v; want %q", test.absolute, relative, err, test.want)
			}
		})
	}
}

func TestAbsolutePathRefusesTraversalAndAbsoluteInput(t *testing.T) {
	root := testRoot
	// 受け取るのはスラッシュ区切りの識別子、返すのはこのファイルシステムの
	// パスである。境界はここにあり、どちらか一方だけを綴り替えると壊れる。
	absolute, err := AbsolutePath(root, "conf.d/10-home.conf")
	if err != nil || absolute != filepath.Join(root, "conf.d", "10-home.conf") {
		t.Fatalf("AbsolutePath = %q, %v", absolute, err)
	}
	for _, relative := range []string{"", ".", "..", "../.bashrc", "conf.d/../../escape", "/etc/passwd", "conf.d//../../x"} {
		if _, err := AbsolutePath(root, relative); !errors.Is(err, ErrExternalPath) {
			t.Errorf("AbsolutePath(%q) error = %v, want ErrExternalPath", relative, err)
		}
	}
}

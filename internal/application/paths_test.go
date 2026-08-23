package application

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func uncleaned(elements ...string) string {
	return strings.Join(elements, string(filepath.Separator))
}

func TestRelativePathRejectsEverythingOutsideTheRoot(t *testing.T) {
	root := testRoot
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

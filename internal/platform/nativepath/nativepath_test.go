package nativepath

import (
	"path/filepath"
	"testing"
)

// device と拡張名前空間の判定は、どの OS で走らせても同じでなければならない。
// Windows で書かれた設定を Unix 側の道具が読んだときにも、同じ理由で同じように
// 拒む必要があるからだ。
func TestSupportedRefusesWin32DeviceAndExtendedNamespacesOnEveryHost(t *testing.T) {
	for _, path := range []string{
		`\\?\C:\Users\A\.ssh\config`,
		`\\.\C:\Users\A\.ssh\config`,
		`\??\C:\Users\A\.ssh\config`,
		`//?/C:/Users/A/.ssh/config`,
		`\\?\UNC\server\share\config`,
		`\\.\PhysicalDrive0`,
	} {
		if Supported(path) {
			t.Errorf("Supported(%q) = true, want false", path)
		}
	}
}

func TestSupportedRefusesEmptyRelativeAndEmbeddedNul(t *testing.T) {
	for _, path := range []string{"", "config", "conf.d/extra.conf", "~/config", "a\x00b"} {
		if Supported(path) {
			t.Errorf("Supported(%q) = true, want false", path)
		}
	}
}

func TestContainsUsesPathBoundariesRatherThanStringPrefixes(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"home", "u", ".ssh")
	tests := map[string]struct {
		candidate string
		want      bool
	}{
		"the root itself":                {root, true},
		"a descendant":                   {filepath.Join(root, "conf.d", "work.conf"), true},
		"a deeper descendant":            {filepath.Join(root, "a", "b", "c"), true},
		"a sibling with the same prefix": {root + "-other", false},
		"the parent":                     {filepath.Dir(root), false},
		"an unrelated tree":              {filepath.Join(string(filepath.Separator)+"etc", "ssh", "ssh_config"), false},
		"an escape through parents":      {filepath.Join(root, "..", "elsewhere"), false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Contains(root, test.candidate); got != test.want {
				t.Fatalf("Contains(%q, %q) = %v, want %v", root, test.candidate, got, test.want)
			}
		})
	}
}

func TestContainsRefusesARelativeCandidate(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"home", "u", ".ssh")
	if Contains(root, "conf.d/work.conf") {
		t.Fatal("a relative candidate was reported inside the root")
	}
}

func TestIdentityCleansBeforeComparing(t *testing.T) {
	root := filepath.Join(string(filepath.Separator)+"home", "u", ".ssh")
	messy := filepath.Join(root, "conf.d", "..", "config")
	if Identity(messy) != Identity(filepath.Join(root, "config")) {
		t.Fatalf("Identity(%q) did not match its cleaned form", messy)
	}
}

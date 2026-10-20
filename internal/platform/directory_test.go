package platform_test

import (
	"errors"
	"path/filepath"
	"testing"

	"sshc/internal/platform"
)

func TestResolveUnderHome(t *testing.T) {
	const home = testHome
	for _, test := range []struct {
		name  string
		given string
		want  string
		err   error
	}{
		{name: "empty stays empty", given: "", want: ""},
		{name: "blank stays empty", given: "   ", want: ""},
		{name: "bare tilde is the home", given: "~", want: home},
		{name: "under the home", given: "~/work", want: filepath.Join(home, "work")},
		{name: "absolute is kept", given: testAbsolute, want: testAbsolute},
		{name: "absolute is cleaned", given: testAbsoluteUncleaned, want: testAbsolute},
		{name: "another user's home", given: "~someone/x", err: platform.ErrDirectoryUser},
		// PowerShell の書き方でも、保存する綴りは同期先の OS で意味が変わるので断る。
		{name: "home followed by a backslash is refused", given: `~\work`, err: platform.ErrDirectoryRelative},
		{name: "relative is refused", given: "work", err: platform.ErrDirectoryRelative},
		{name: "dot is refused", given: ".", err: platform.ErrDirectoryRelative},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := platform.ResolveUnderHome(test.given, home)
			if !errors.Is(err, test.err) {
				t.Fatalf("error = %v, want %v", err, test.err)
			}
			if err == nil && got != test.want {
				t.Fatalf("ResolveUnderHome(%q) = %q, want %q", test.given, got, test.want)
			}
		})
	}
}

func TestATypedPathStartsAtHomeOnlyWithTildeAndASeparator(t *testing.T) {
	separator := string(filepath.Separator)
	for typed, want := range map[string]string{"~": "", "~/": "", "~/logs": "logs", "~" + separator + "x": "x"} {
		belowHome, ok := platform.CutTypedHome(typed)
		if !ok || belowHome != want {
			t.Errorf("CutTypedHome(%q) = %q, %v; want %q, true", typed, belowHome, ok, want)
		}
	}
	for _, typed := range []string{"", "~user", "~user/logs", "logs/~", "/home/~"} {
		if _, ok := platform.CutTypedHome(typed); ok {
			t.Errorf("CutTypedHome(%q) started at home", typed)
		}
	}
}

func TestExpandTypedHomeJoinsTheHomeAndLeavesOtherPathsAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	expanded, err := platform.ExpandTypedHome("~/dl/a.log")
	if err != nil || expanded != filepath.Join(home, "dl", "a.log") {
		t.Errorf("ExpandTypedHome(~/dl/a.log) = %q, %v", expanded, err)
	}
	for _, typed := range []string{"relative/x", "~user/x", filepath.Join(home, "x")} {
		if unchanged, err := platform.ExpandTypedHome(typed); err != nil || unchanged != typed {
			t.Errorf("ExpandTypedHome(%q) = %q, %v; want it unchanged", typed, unchanged, err)
		}
	}
}

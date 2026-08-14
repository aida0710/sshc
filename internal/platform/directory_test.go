package platform_test

import (
	"errors"
	"testing"

	"sshc/internal/platform"
)

func TestResolveUnderHome(t *testing.T) {
	const home = "/home/tester"
	for _, test := range []struct {
		name  string
		given string
		want  string
		err   error
	}{
		{name: "empty stays empty", given: "", want: ""},
		{name: "blank stays empty", given: "   ", want: ""},
		{name: "bare tilde is the home", given: "~", want: home},
		{name: "under the home", given: "~/work", want: "/home/tester/work"},
		{name: "absolute is kept", given: "/srv/deploy", want: "/srv/deploy"},
		{name: "absolute is cleaned", given: "/srv//deploy/../deploy/", want: "/srv/deploy"},
		{name: "another user's home", given: "~someone/x", err: platform.ErrDirectoryUser},
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

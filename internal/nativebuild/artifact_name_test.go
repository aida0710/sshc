package nativebuild

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// goOperatingSystems is every GOOS that `go tool dist list` names. A target that
// nativebuild may build for must also pass the artifact name check, or the
// release job builds the binary and then stops on its name (v0.59.0 did, for
// FreeBSD and OpenBSD).
var goOperatingSystems = []string{
	"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js",
	"linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows",
}

func TestTheArtifactNameCheckAcceptsExactlyTheOperatingSystemsThatAreBuilt(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no POSIX shell on this host; the artifact name check runs in a step that has one")
	}
	script := filepath.Join("..", "..", "scripts", "verify-artifact-name.sh")
	for _, goos := range goOperatingSystems {
		t.Run(goos, func(t *testing.T) {
			name := "sshc-" + goos + "-amd64"
			if goos == "windows" {
				name += ".exe"
			}
			// The script reads the name after the last "/", as the release jobs pass it, on every host.
			output, err := exec.Command("sh", script, "dist/"+name, goos, "amd64").CombinedOutput()
			if accepted, built := err == nil, supportedTargetOS(goos); accepted != built {
				t.Fatalf("the name check accepts %s: %v, nativebuild builds it: %v\n%s", goos, accepted, built, output)
			}
		})
	}
}

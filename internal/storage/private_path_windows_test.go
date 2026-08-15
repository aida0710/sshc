//go:build windows

package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsPrivateStateContainmentIsCaseInsensitive(t *testing.T) {
	for _, state := range []string{
		filepath.Join(`C:\Users\Aida\.ssh`, "sshc"),
		filepath.Join(`\\Server\Share\Users\Aida\.ssh`, "sshc"),
	} {
		t.Run(state, func(t *testing.T) {
			for name, path := range map[string]string{
				"same directory": strings.ToUpper(state),
				"descendant":     strings.ToUpper(filepath.Join(state, "trash", "entry", "id_work")),
			} {
				t.Run(name, func(t *testing.T) {
					if !privateStateContains(state, path) {
						t.Fatalf("privateStateContains(%q, %q) = false", state, path)
					}
				})
			}
			if privateStateContains(state, strings.ToUpper(state)+"-outside") {
				t.Fatal("case-insensitive prefix without a path boundary was accepted")
			}
		})
	}
}

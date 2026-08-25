//go:build !windows

package nativepath

import (
	"errors"
	"os"
	"testing"
)

func TestResolveRootAliasRejectsRelativePaths(t *testing.T) {
	if _, err := ResolveRootAlias("var/folders/workspace"); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("ResolveRootAlias(relative) error = %v, want os.ErrInvalid", err)
	}
}

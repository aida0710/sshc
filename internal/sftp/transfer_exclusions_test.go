package sftp

import (
	"errors"
	"strings"
	"testing"
)

func TestTransferExclusionsMatchNamesAtAnyDepthAndRelativePathsAtTheRoot(t *testing.T) {
	patterns := transferExclusions{".git", "node_modules", "*.log", "build/cache", "one/?.txt"}
	for _, relative := range []string{".git/config", "nested/.git", "nested/node_modules/pkg/index.js", "debug.log", "deep/debug.log", "build/cache/generated", "one/文.txt"} {
		if !patterns.excludes(relative) {
			t.Errorf("%q was not excluded", relative)
		}
	}
	for _, relative := range []string{"", ".", "/", "git/config", "src/main.go", "deep/build/cache/generated", "one/two.txt"} {
		if patterns.excludes(relative) {
			t.Errorf("%q unexpectedly excluded", relative)
		}
	}
}

func TestTransferExclusionsRejectUnsupportedOrUnboundedPatterns(t *testing.T) {
	for _, pattern := range []string{"", "../secret", "a/../b", "/root", "a/", "a//b", "**", "*.log\n*", "[abc]", "!keep", "a\\b", " leading", "trailing ", strings.Repeat("a", MaxTransferExclusionPatternBytes+1)} {
		if err := ValidateTransferExclusionPatterns([]string{pattern}); !errors.Is(err, ErrInvalidTransfer) {
			t.Errorf("pattern %q = %v", pattern, err)
		}
	}
	if err := ValidateTransferExclusionPatterns(make([]string, MaxTransferExclusionPatterns+1)); !errors.Is(err, ErrInvalidTransfer) {
		t.Fatal(err)
	}
	if err := ValidateTransferExclusionPatterns([]string{".git", "*.log", "build/cache", "a?b"}); err != nil {
		t.Fatal(err)
	}
}

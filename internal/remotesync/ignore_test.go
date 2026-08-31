package remotesync_test

import (
	"errors"
	"strings"
	"testing"

	"sshc/internal/remotesync"
)

func TestDefaultIgnoreRulesCoverPortableNoise(t *testing.T) {
	rules, err := remotesync.CompileIgnoreRules([]byte(remotesync.DefaultIgnoreDocument))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		".DS_Store", "connections/work/.DS_Store", "Thumbs.db",
		"keys/group/desktop.ini", "config.bak", "nested/config.tmp", "edit.lock",
	} {
		if !rules.Match(path) {
			t.Errorf("default rules do not ignore %q", path)
		}
	}
	for _, path := range []string{
		"config", "known_hosts", "keys/work/id_ed25519", remotesync.IgnorePath,
		remotesync.TravelPath, remotesync.SnippetsPath,
	} {
		if rules.Match(path) {
			t.Errorf("default rules ignore %q", path)
		}
	}
}

func TestBroadIgnoreRuleCannotDisableLogicalSecretDocuments(t *testing.T) {
	rules, err := remotesync.CompileIgnoreRules([]byte("*\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{remotesync.IgnorePath, remotesync.TravelPath, remotesync.SnippetsPath} {
		if rules.Match(path) {
			t.Errorf("control path %q was ignored", path)
		}
	}
}

func TestIgnoreRulesUseLastMatchAndGitignoreWildcards(t *testing.T) {
	rules, err := remotesync.CompileIgnoreRules([]byte("cache/\n*.tmp\n!keep.tmp\nkeys/**/scratch?.bin\n"))
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string]bool{
		"cache/data": true, "nested/cache/data": true,
		"nested/file.tmp": true, "nested/keep.tmp": false,
		"keys/a/b/scratch1.bin": true, "keys/a/b/scratch12.bin": false,
	}
	for path, want := range checks {
		if got := rules.Match(path); got != want {
			t.Errorf("Match(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestIgnoreRulesRejectMalformedOrOversizedDocuments(t *testing.T) {
	for _, document := range [][]byte{
		[]byte("broken[\n"),
		[]byte("[z-a]\n"),
		[]byte("trailing\\\n"),
		[]byte("!\n"),
		[]byte(strings.Repeat("x", remotesync.MaxIgnoreBytes+1)),
	} {
		if _, err := remotesync.CompileIgnoreRules(document); !errors.Is(err, remotesync.ErrInvalidIgnoreRules) {
			t.Errorf("CompileIgnoreRules(%q) = %v", document[:min(len(document), 20)], err)
		}
	}
}

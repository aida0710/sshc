package app

import (
	"testing"

	"sshc/internal/effective"
	"sshc/internal/textencoding"
)

func TestSFTPPoolIdentityTracksConnectionAndJumpSettings(t *testing.T) {
	for _, change := range []struct {
		alias, keyword, value string
	}{
		{"edge", "hostname", "other.example"},
		{"edge", "user", "other-user"},
		{"edge", "port", "2222"},
		{"edge", "identityfile", "~/.ssh/other-key"},
		{"edge", "proxyjump", "other-jump"},
		{"jump", "hostname", "other-jump.example"},
	} {
		t.Run(change.alias+"/"+change.keyword, func(t *testing.T) {
			settings := map[string]effective.Values{
				"edge": {Entries: map[string][]string{
					"hostname": {"edge.example"}, "user": {"operator"}, "port": {"22"}, "proxyjump": {"jump"},
				}},
				"jump":       {Entries: map[string][]string{"hostname": {"jump.example"}, "user": {"operator"}}},
				"other-jump": {Entries: map[string][]string{"hostname": {"other-jump.example"}, "user": {"operator"}}},
			}
			parts := sshParts{
				home:     t.TempDir(),
				resolve:  func(alias string) (effective.Values, error) { return settings[alias], nil },
				encoding: func(string) (textencoding.Name, error) { return textencoding.UTF8, nil },
			}
			resolve := parts.sftp()
			before, err := resolve(t.Context(), "edge")
			if err != nil {
				t.Fatal(err)
			}
			unchanged, err := resolve(t.Context(), "edge")
			if err != nil || unchanged.Identity != before.Identity {
				t.Fatalf("identical settings do not reuse the same pool: %v", err)
			}
			settings[change.alias].Entries[change.keyword] = []string{change.value}
			after, err := resolve(t.Context(), "edge")
			if err != nil {
				t.Fatal(err)
			}
			if before.Identity == after.Identity {
				t.Fatal("changed connection settings still address the old pool")
			}
		})
	}
}

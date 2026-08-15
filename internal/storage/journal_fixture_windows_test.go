//go:build windows

package storage

import "testing"

func assertLoadedJournalFixturePolicy(t *testing.T, path string) {
	t.Helper()
	assertWindowsPrivatePath(t, path)
}

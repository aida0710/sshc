//go:build !windows

package storage

import "testing"

func TestUnixPrivateStateContainmentRemainsCaseSensitive(t *testing.T) {
	state := "/home/aida/.ssh/sshc"
	if !privateStateContains(state, state+"/trash/entry") {
		t.Fatal("exact-case descendant was rejected")
	}
	if privateStateContains(state, "/home/aida/.ssh/SSHC/trash/entry") {
		t.Fatal("case-different Unix path was treated as private state")
	}
}

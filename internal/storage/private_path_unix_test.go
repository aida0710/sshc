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

func TestUnixLoadedJournalClaimsRemainCaseSensitive(t *testing.T) {
	claimed := []string{"/home/aida/.ssh/keys/id_work"}
	if journalPathAlreadyClaimed(claimed, "/home/aida/.ssh/keys/id_other") {
		t.Fatal("different Unix path was treated as already claimed")
	}
	if journalPathAlreadyClaimed(claimed, "/home/aida/.ssh/KEYS/ID_WORK") {
		t.Fatal("Unix case alias was treated as the same path")
	}
	if !journalPathAlreadyClaimed(claimed, claimed[0]) {
		t.Fatal("exact Unix path was not treated as already claimed")
	}
}

package sftp

import (
	"errors"
	"testing"
)

func TestPublishedNamesTreatCaseAndUnicodeNormalizationAsOneName(t *testing.T) {
	published := newPublishedNames()
	published.record("/inbox/café.txt") // NFC: é as one code point
	for _, spelling := range []string{"/inbox/CAFÉ.txt", "/inbox/café.txt"} {
		if err := published.existingEntryError(spelling, true); !errors.Is(err, ErrNameCollision) {
			t.Fatalf("%q: err = %v, want a name collision", spelling, err)
		}
	}
}

func TestPublishedNamesLeaveEntriesTheRunDidNotWriteToTheOverwriteApproval(t *testing.T) {
	published := newPublishedNames()
	published.record("/inbox/a.txt")
	if err := published.existingEntryError("/other/A.txt", true); err != nil {
		t.Fatalf("same name in another folder: err = %v", err)
	}
	if err := published.existingEntryError("/inbox/b.txt", false); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("unapproved existing entry: err = %v", err)
	}
	if err := published.existingEntryError("/inbox/b.txt", true); err != nil {
		t.Fatalf("approved existing entry: err = %v", err)
	}
}

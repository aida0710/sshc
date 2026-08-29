package terminal_test

import (
	"bytes"
	"testing"

	"sshc/internal/terminal"
)

func TestRingKeepsTheNewestBytesWhenItWraps(t *testing.T) {
	tests := []struct {
		name     string
		capacity int
		writes   []string
		want     string
	}{
		{"below the limit", 8, []string{"ab", "cd"}, "abcd"},
		{"exactly the limit", 4, []string{"ab", "cd"}, "abcd"},
		{"one byte over", 4, []string{"ab", "cde"}, "bcde"},
		{"a single write longer than the buffer", 4, []string{"abcdefg"}, "defg"},
		{"many wraps", 3, []string{"a", "b", "c", "d", "e", "f", "g"}, "efg"},
		{"nothing written", 4, nil, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ring := terminal.NewRing(test.capacity)
			for _, write := range test.writes {
				written, err := ring.Write([]byte(write))
				if err != nil || written != len(write) {
					t.Fatalf("Write(%q) = %d, %v", write, written, err)
				}
			}
			if got := string(ring.Snapshot()); got != test.want {
				t.Fatalf("Snapshot() = %q, want %q", got, test.want)
			}
			if ring.Len() != len(test.want) {
				t.Fatalf("Len() = %d, want %d", ring.Len(), len(test.want))
			}
		})
	}
}

func TestRingSnapshotDoesNotAliasTheBuffer(t *testing.T) {
	ring := terminal.NewRing(4)
	if _, err := ring.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	first := ring.Snapshot()
	if _, err := ring.Write([]byte("ef")); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, []byte("abcd")) {
		t.Fatalf("the earlier snapshot changed to %q", first)
	}
	if got := string(ring.Snapshot()); got != "cdef" {
		t.Fatalf("Snapshot() = %q", got)
	}
}

func TestRingReadUsesAMonotonicCursorAcrossWraps(t *testing.T) {
	ring := terminal.NewRing(5)
	_, _ = ring.Write([]byte("abcd"))
	first, ok := ring.ReadFrom(0, 3)
	if !ok || string(first.Data) != "abc" || first.Start != 0 || first.Next != 3 || first.End != 4 || first.Truncated {
		t.Fatalf("first read = %#v, %v", first, ok)
	}
	_, _ = ring.Write([]byte("efgh"))
	second, ok := ring.ReadFrom(first.Next, 10)
	if !ok || string(second.Data) != "defgh" || second.Start != 3 || second.Next != 8 || second.End != 8 || second.Truncated {
		t.Fatalf("second read = %#v, %v", second, ok)
	}
	old, ok := ring.ReadFrom(0, 2)
	if !ok || string(old.Data) != "de" || old.Start != 3 || old.Next != 5 || !old.Truncated {
		t.Fatalf("truncated read = %#v, %v", old, ok)
	}
	if _, ok := ring.ReadFrom(9, 1); ok {
		t.Fatal("a cursor beyond the end was accepted")
	}
}

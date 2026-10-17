package terminal

import (
	"strings"
	"testing"
)

func indexed(output string) *controlStringIndex {
	var index controlStringIndex
	index.Write([]byte(output))
	return &index
}

func TestALongControlStringCutByTheBufferIsSkipped(t *testing.T) {
	image := "\x1b]1337;File=inline=1:" + strings.Repeat("A", 1000) + "\a"
	index := indexed("before" + image + "after")
	end := uint64(len("before" + image))
	for _, oldest := range []uint64{7, 100, end - 1} {
		if got := index.SafeStart(oldest); got != end {
			t.Fatalf("SafeStart(%d) = %d, want %d", oldest, got, end)
		}
	}
}

func TestAReplayStartingOutsideAControlStringIsKept(t *testing.T) {
	image := "\x1b]1337;File=inline=1:" + strings.Repeat("A", 1000) + "\a"
	index := indexed("before" + image + "after")
	for _, oldest := range []uint64{0, 6, uint64(len("before" + image)), uint64(len("before" + image + "af"))} {
		if got := index.SafeStart(oldest); got != oldest {
			t.Fatalf("SafeStart(%d) = %d", oldest, got)
		}
	}
}

func TestShortControlStringsAreNotRemembered(t *testing.T) {
	index := indexed("\x1b]0;title\a\x1b]7;file:///home/user\a")
	if len(index.spans) != 0 {
		t.Fatalf("spans = %v", index.spans)
	}
}

func TestEachKindOfControlStringEndsAtItsOwnTerminator(t *testing.T) {
	payload := strings.Repeat("#0;2;0;0;0", 40)
	for name, test := range map[string]struct {
		output string
		end    int
	}{
		"DCS ended by ST":         {"\x1bPq" + payload + "\x1b\\tail", len("\x1bPq" + payload + "\x1b\\")},
		"DCS keeps BEL as data":   {"\x1bPq" + payload + "\a" + payload + "\x1b\\", len("\x1bPq" + payload + "\a" + payload + "\x1b\\")},
		"APC ended by ST":         {"\x1b_G" + payload + "\x1b\\", len("\x1b_G" + payload + "\x1b\\")},
		"OSC ended by BEL":        {"\x1b]1337;" + payload + "\a", len("\x1b]1337;" + payload + "\a")},
		"CAN aborts the string":   {"\x1bPq" + payload + "\x18tail", len("\x1bPq" + payload + "\x18")},
		"a new ESC ends a string": {"\x1b]1337;" + payload + "\x1b[0m", len("\x1b]1337;" + payload)},
	} {
		index := indexed(test.output)
		if len(index.spans) != 1 || index.spans[0] != (controlStringSpan{start: 0, end: uint64(test.end)}) {
			t.Fatalf("%s: spans = %v, want end %d", name, index.spans, test.end)
		}
	}
}

func TestStringsThatLeftTheBufferAreForgotten(t *testing.T) {
	image := "\x1b]1337;File=:" + strings.Repeat("A", 1000) + "\a"
	index := indexed(image + image)
	index.Forget(uint64(len(image)))
	if len(index.spans) != 1 || index.spans[0].start != uint64(len(image)) {
		t.Fatalf("spans = %v", index.spans)
	}
}

func TestALongStringStillArrivingSkipsEverythingRetained(t *testing.T) {
	partial := "\x1b]1337;File=:" + strings.Repeat("A", 1000)
	index := indexed(partial)
	if got := index.SafeStart(10); got != uint64(len(partial)) {
		t.Fatalf("SafeStart = %d, want %d", got, len(partial))
	}
}

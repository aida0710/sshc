package vt

import (
	"strings"
	"testing"

	"sshc/internal/testwait"
)

// largestCount は、パーサーが渡す回数の一番大きい値である。回数は 31 ビットに切られ、2^31-1 は「省いた」の印に
// 当たる。
const largestCount = "2147483646"

// 回数が画面よりずっと大きくても、エミュレーターはすぐに読み終える。回数のとおりに繰り返すと、この出力を
// 読むのに何分もかかる。
func TestACountFarPastTheScreenIsReadAtOnce(t *testing.T) {
	const sequences = 100
	for _, sequence := range []string{
		"\x1b[" + largestCount + "I", "\x1b[" + largestCount + "Z", "\x1b[" + largestCount + "X", "a\x1b[" + largestCount + "b",
	} {
		for name, newEmulator := range map[string]func(int, int) *SnapshotEmulator{
			"snapshot": NewSnapshotEmulator, "leaving lines": NewLeavingLinesEmulator,
		} {
			output := []byte("\x1b[12;40H" + strings.Repeat(sequence, sequences))
			read := make(chan struct{})
			go func() {
				defer close(read)
				_, _ = newEmulator(80, 24).Write(output)
			}()
			if !testwait.Reached(func() bool {
				select {
				case <-read:
					return true
				default:
					return false
				}
			}) {
				t.Fatalf("%s: %d of %q are still being read", name, sequences, sequence)
			}
		}
	}
}

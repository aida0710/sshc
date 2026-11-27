package vt

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
)

// repeatSetups は、REP の前に置く出力の部品である。カーソルの位置、余白、折り返し、文字の集合、色、画面の切り替え
// を混ぜ、REP が字を打つのと同じに振る舞うかを、いろいろな状態で確かめる。withTopMargin が偽なら、上の余白を
// 1 行目より下に置かない。文字の集合で写した字は上流の index で流れ、上流はそのとき画面の途中の行もログへ
// 押し出すので、ログの行を比べる試験はその形を除く。
func repeatSetups(withTopMargin bool) []func(*rand.Rand, int, int) string {
	return []func(*rand.Rand, int, int) string{
		func(r *rand.Rand, width, height int) string {
			return fmt.Sprintf("\x1b[%d;%dH", 1+r.IntN(height), 1+r.IntN(width))
		},
		func(r *rand.Rand, _, height int) string {
			top := 1
			if withTopMargin {
				top += r.IntN(height)
			}
			return fmt.Sprintf("\x1b[%d;%dr", top, top+1+r.IntN(height))
		},
		func(*rand.Rand, int, int) string { return "\x1b[r" },
		func(r *rand.Rand, width, _ int) string {
			return fmt.Sprintf("\x1b[?69h\x1b[%d;%ds", 1+r.IntN(width), 2+r.IntN(width))
		},
		func(*rand.Rand, int, int) string { return "\x1b[?69l" },
		func(*rand.Rand, int, int) string { return "\x1b[?7l" },
		func(*rand.Rand, int, int) string { return "\x1b[?7h" },
		func(*rand.Rand, int, int) string { return "\x1b[?6h" },
		func(*rand.Rand, int, int) string { return "\x1b[?6l" },
		func(*rand.Rand, int, int) string { return "\x1b(0" },
		func(*rand.Rand, int, int) string { return "\x1b(B" },
		func(*rand.Rand, int, int) string { return "\x1b*0\x8e" },
		func(r *rand.Rand, _, _ int) string { return fmt.Sprintf("\x1b[4%dm", r.IntN(8)) },
		func(*rand.Rand, int, int) string { return "\x1b[0m" },
		func(*rand.Rand, int, int) string { return "\x1b[?1049h" },
		func(*rand.Rand, int, int) string { return "\x1b[?1049l" },
		func(r *rand.Rand, width, _ int) string { return strings.Repeat("x", r.IntN(2*width)) },
		func(*rand.Rand, int, int) string { return "\r\n" },
		func(*rand.Rand, int, int) string { return "日" },
	}
}

// repeatCounts は、試す REP の回数である。1、行の幅の前後、画面のマス目の前後、それよりずっと多い回数を混ぜる。
func repeatCounts(r *rand.Rand, width, height int) []int {
	cells := width * height
	return []int{
		1, 2, max(1, width-1), width, width + 1, max(1, cells-1), cells, cells + 1,
		cells + r.IntN(4*cells), 3*cells + r.IntN(width), 7*cells + r.IntN(20*cells),
	}
}

// expandLeftLines は、続けて同じ行が流れたことを Repeats で表した行を、1行ずつに戻す。
func expandLeftLines(lines []LeftLine) []string {
	var texts []string
	for _, line := range lines {
		for range 1 + line.Repeats {
			texts = append(texts, line.Text)
		}
	}
	return texts
}

// REP（CSI Ps b）は、直前の字を Ps 回打つのと同じである。画面より長い回数を減らして打っても、画面、カーソル、
// 折り返しを待つ状態、流れた行は、字を1つずつ打ったときと同じでなければならない。
func TestRepeatingACharacterDrawsWhatTypingItDraws(t *testing.T) {
	// エミュレーターを作るたびに上流が 4 MiB を取るので、1回に試す回数を絞り、そのぶん状態を多く試す。
	const rounds, countsPerRound = 300, 4
	emulators := []struct {
		name  string
		build func(int, int) *SnapshotEmulator
	}{{"snapshot", NewSnapshotEmulator}, {"leaving lines", NewLeavingLinesEmulator}}
	random := rand.New(rand.NewPCG(3, 5))
	for round := range rounds {
		width, height := 1+random.IntN(9), 1+random.IntN(7)
		emulator := emulators[round%len(emulators)]
		withTopMargin := round/len(emulators)%2 == 0
		setups := repeatSetups(withTopMargin)
		var setup strings.Builder
		for range random.IntN(8) {
			setup.WriteString(setups[random.IntN(len(setups))](random, width, height))
		}
		character := []string{"a", "q", " "}[random.IntN(3)]
		// REP の前の1字だけの切り替え（SS2。C1 の 0x8e）は、REP が打つ最初の字に効く。
		shift := []string{"", "\x1b*0\x8e"}[random.IntN(2)]
		counts := repeatCounts(random, width, height)
		for range countsPerRound {
			count := counts[random.IntN(len(counts))]
			repeated := setup.String() + character + shift + fmt.Sprintf("\x1b[%db", count) + "\u0301z"
			typed := setup.String() + character + shift + strings.Repeat(character, count) + "\u0301z"
			got, want := emulator.build(width, height), emulator.build(width, height)
			writeAll(t, got, repeated)
			writeAll(t, want, typed)
			if gotState, wantState := got.Snapshot(), want.Snapshot(); gotState != wantState {
				t.Fatalf("%s %dx%d: %q leaves\n%q\nwant what %q leaves\n%q", emulator.name, width, height, repeated, gotState, typed, wantState)
			}
			if withTopMargin {
				continue
			}
			if gotLines, wantLines := expandLeftLines(got.TakeLeftLines()), expandLeftLines(want.TakeLeftLines()); !slices.Equal(gotLines, wantLines) {
				t.Fatalf("%s %dx%d: %q leaves %d lines %q, want the %d lines %q leaves %q",
					emulator.name, width, height, repeated, len(gotLines), gotLines, len(wantLines), typed, wantLines)
			}
		}
	}
}

// 回数の大きい REP を何百も読んでも、エミュレーターが書く字は、回数ではなく画面の大きさで決まる。エンジンは
// セッションのロックを持ったまま出力を読むので、少ない出力で長く止まってはならない。
func TestRepeatingFarPastTheScreenWritesNoMoreThanTheScreenHolds(t *testing.T) {
	const width, height, repeats = 80, 24, 300
	output := strings.Repeat("a\x1b[65535b", repeats)
	for name, newEmulator := range map[string]func(int, int) *SnapshotEmulator{
		"snapshot": NewSnapshotEmulator, "leaving lines": NewLeavingLinesEmulator,
	} {
		emulator := newEmulator(width, height)
		cursorMoves := 0
		emulator.cb.CursorPosition = func(uv.Position, uv.Position) { cursorMoves++ }
		writeAll(t, emulator, output)
		// 1つの REP で、画面のマス目の2倍より多く字を書かない。
		if limit := repeats * 2 * width * height; cursorMoves > limit {
			t.Fatalf("%s: %d REPs moved the cursor %d times, want at most %d", name, repeats, cursorMoves, limit)
		}
	}
}

// 画面より長い REP で同じ行が何度も流れても、ログにはその行を1つと、続いた回数だけを残す。
func TestARepeatLongerThanTheScreenLeavesTheSameLineOnceWithItsCount(t *testing.T) {
	emulator := NewLeavingLinesEmulator(5, 2)
	writeAll(t, emulator, "top\r\na\x1b[65535b")
	left := emulator.TakeLeftLines()
	// 流れる 13107 行を、1行ずつは持たない。
	if len(left) > 10 {
		t.Fatalf("kept %d lines for one REP, want the repeated line kept once with a count", len(left))
	}
	// 65536 字は 5 字ずつの 13108 行（最後の行は1字）になり、画面に残る 2 行のほかは流れる。
	want := append([]string{"top"}, slices.Repeat([]string{"aaaaa"}, 13108-2)...)
	if got := expandLeftLines(left); !slices.Equal(got, want) {
		t.Fatalf("left %d lines, want %d: %q", len(got), len(want), got[:min(len(got), 8)])
	}
}

// 1回の読み取りで、そのバイト数より多くの行が流れても（SU で何度も画面を流す）、ログの行は落ちない。
func TestALongReadKeepsEveryLineThatLeft(t *testing.T) {
	const height, scrolls = 24, 3000
	emulator := NewLeavingLinesEmulator(20, height)
	var numbered []string
	for row := 1; row <= height; row++ {
		numbered = append(numbered, fmt.Sprint(row))
	}
	writeAll(t, emulator, strings.Join(numbered, "\r\n")+strings.Repeat("\x1b[99S", scrolls))
	left := expandLeftLines(emulator.TakeLeftLines())
	if len(left) != height*scrolls || !slices.Equal(left[:height], numbered) {
		t.Fatalf("left %d lines starting %q, want %d starting with the numbered rows", len(left), left[:min(len(left), 3)], height*scrolls)
	}
}

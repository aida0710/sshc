package vt

// sshc がこのパッケージの写しに足したもの（README.sshc.md）。REP（CSI Ps b）を、画面の大きさまでの仕事で描く。

import "github.com/charmbracelet/x/ansi"

// repeatSettlingRows は、REP が打つ同じ字で画面の行が埋まりきるのに、画面の高さより多く要る行の数である。
// 今の行の残りに1行、余白の中の行を流しきるのに1行、余裕に1行を見ている。
const repeatSettlingRows = 3

// repeatPreviousCharacterOnScreen は、REP を、直前の字を Ps 回打つのと同じに描く。上流は回数のとおりに1字ずつ
// 打つので、65535 回を頼む8バイトで 65535 字を書く。エンジンはセッションのロックを持ったまま出力を読むので、
// 画面の見た目が変わらない分は打たない（repeatedCellsOnScreen）。打つ字は、文字の集合で写したものを
// putNarrowCell で書く。上流の handleGrapheme は、折り返しで流すたびに余白の中のマスを1つずつ動かす。
func (e *SnapshotEmulator) repeatPreviousCharacterOnScreen(params ansi.Params) bool {
	count, _, _ := params.Param(0, 1)
	character := e.lastChar
	if character == 0 || count <= 0 {
		return true
	}
	// 上流と同じく、最後の1字は書かずに持っておく。続く結合文字がその字に付くためである。
	cells := count - 1
	if cells > 0 && e.gsingle != 0 {
		// 1字だけの文字の集合の切り替え（SS2・SS3）は、最初の1字にだけ効く。
		e.handleGrapheme(string(character), 1)
		cells--
	}
	e.putRepeatedCells(character, cells)
	e.handlePrint(character)
	return true
}

// putRepeatedCells は、字 character を cells 回書くのと同じ画面にする。character は1バイトの字である
// （handleGrapheme が lastChar に残すのはそれだけ）。
func (e *SnapshotEmulator) putRepeatedCells(character rune, cells int) {
	content := string(character)
	if mapped, ok := e.charsets[e.gl][byte(character)]; ok {
		content = mapped
	}
	width := e.Width()
	written, skippedRows := repeatedCellsOnScreen(cells, width, e.Height())
	if skippedRows == 0 {
		e.putNarrowCells(content, written)
		return
	}
	// 減らした行のそれぞれで流れたはずの行は、最後の1行分を書くあいだに流れる行と同じである。1行分の字では
	// 折り返しは1度なので、流れるのは多くて1行である。
	e.putNarrowCells(content, written-width)
	e.moveLeftLines()
	e.putNarrowCells(content, width)
	if kept := e.scrs[0].scrollback; kept != nil && len(kept.lines) > 0 {
		e.moveLeftLines()
		e.left[len(e.left)-1].Repeats += skippedRows
	}
}

func (e *SnapshotEmulator) putNarrowCells(content string, count int) {
	for range count {
		e.putNarrowCell(content)
	}
}

// repeatedCellsOnScreen は、画面に同じ字を cells 回書くのと同じ画面になる、書く字の数と、減らした行の数を返す。
// 画面のマス目より多く書くと、書いている行と余白の中の行はどれも同じ字で埋まり、そこからは width 字ごとに
// 同じ画面に戻る（1行流れ、新しい行がまた同じ字で埋まる）。そのため、埋めきるのに足りる字数より多い分は、
// width 字の倍数だけ減らしてよい。
func repeatedCellsOnScreen(cells, width, height int) (written, skippedRows int) {
	settled := width * (height + repeatSettlingRows)
	if cells <= settled {
		return cells, 0
	}
	skippedRows = (cells - settled) / width
	return cells - skippedRows*width, skippedRows
}

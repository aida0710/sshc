package effective

import (
	"sshc/internal/config"
	"sshc/internal/sshmatch"
)

// blockState は、読んでいる行にいま効いているブロックの一致の状態である。
//
// OpenSSH の readconf.c が持つ *activep に当たる。Include はこの状態を取り込み先へ
// 引き継ぎ、戻ったら Include 行の時点の状態に戻す（readconf.c の
// `*activep = oactive`）。
type blockState struct {
	// applies は、この行がいま解決している alias に効くか。
	applies bool
	// kind は、効いているブロックがどう一致したか（SourceGlobal など）。
	kind string
	// condition は、効いているブロックの見出し。最上位のグローバルでは空。
	condition string
}

// blockHeader は、走査が出会った Host / Match の見出しひとつである。
type blockHeader struct {
	path      string
	block     config.Block
	condition string
	// neverMatch は、一致していないブロックの中の Include から読まれたことを示す。
	// OpenSSH はそのファイルの Host / Match をどれも一致させない
	// （SSHCONF_NEVERMATCH）。それでも Match exec のコマンドは実行するので、
	// 見出しそのものは知らせる。
	neverMatch bool
}

// number は見出しの行番号（1 始まり）である。
func (header blockHeader) number() int { return header.block.Header + 1 }

// directiveLine は、効いているブロックの中のディレクティブひとつである。
type directiveLine struct {
	path string
	// number は 1 始まりの行番号。
	number int
	line   config.Line
	state  blockState
}

// loadOrderVisitor は、walkLoadOrder が読み込み順に呼ぶ先である。
type loadOrderVisitor struct {
	// enterBlock は、Host / Match の見出しでそのブロックが alias に効くかを決める。
	// neverMatch の見出しでも呼ぶが、戻り値にかかわらずブロックは効かない。
	enterBlock func(header blockHeader) (kind string, applies bool)
	// directive は、効いているブロックのディレクティブごとに呼ぶ。Include 行は
	// 走査が自分で辿るので渡さない。
	directive func(directive directiveLine)
}

// walkLoadOrder は、設定を OpenSSH が読む順に訪れる。
//
// 読み込み順はファイル順ではない。OpenSSH は Include をその行のある位置で読むので、
// Include より下に書かれた行は、取り込んだファイル全体のあとで読まれる。そして
// 最初の値が勝つので、取り込んだファイルの方が勝つ。
//
// 一致していないブロックの中の Include も読む。OpenSSH もそうする。取り込み先の
// 先頭の見出しのない行は取り込み元の状態に従い、Host / Match はどれも一致しない。
// Include から戻ったあとは、Include 行の時点の状態で続ける。取り込み先の最後の
// Host の判定を持ち越すと、取り込み元の後ろの行が別の alias に効いたり落ちたりする。
//
// chain は循環を止める。二度取り込まれたファイルは二度読む。OpenSSH がそうする
// からだ。二度目の読みは何も寄与しない。最初の値がすでに取られているからである。
func walkLoadOrder(graph *config.Graph, visitor loadOrderVisitor) {
	if graph == nil {
		return
	}
	walker := loadOrderWalker{graph: graph, visitor: visitor, chain: map[string]bool{}}
	walker.readFile(graph.Root, blockState{applies: true, kind: SourceGlobal}, false)
}

type loadOrderWalker struct {
	graph   *config.Graph
	visitor loadOrderVisitor
	chain   map[string]bool
}

// readingFile は、走査がいま読んでいるファイルである。
type readingFile struct {
	path string
	node *config.Node
	// neverMatch は、一致していないブロックの中の Include から読まれたことを示す。
	// OpenSSH はこのファイルの Host / Match をどれも一致させない（SSHCONF_NEVERMATCH）。
	neverMatch bool
}

// readFile は、ファイルひとつを先頭から読む。inherited は取り込み元の Include 行の
// 時点の状態で、ファイル先頭の見出しのない行はこれに従う。
func (w *loadOrderWalker) readFile(filePath string, inherited blockState, neverMatch bool) {
	node := w.graph.Nodes[filePath]
	if node == nil || node.File == nil || w.chain[filePath] {
		return
	}
	w.chain[filePath] = true
	defer delete(w.chain, filePath)

	current := readingFile{path: filePath, node: node, neverMatch: neverMatch}
	blocks := node.File.Blocks()
	position := 0
	state := inherited
	for index, line := range node.File.Lines {
		if position+1 < len(blocks) && blocks[position+1].Header == index {
			position++
			state = w.enterBlock(current, blocks[position])
			continue
		}
		if line.Kind != config.LineDirective {
			continue
		}
		if config.EqualKeyword(line.Keyword, "Include") {
			w.readIncludes(current, index+1, state)
			continue
		}
		if state.applies {
			w.visitor.directive(directiveLine{path: filePath, number: index + 1, line: line, state: state})
		}
	}
}

func (w *loadOrderWalker) enterBlock(current readingFile, block config.Block) blockState {
	header := blockHeader{
		path: current.path, block: block, condition: current.node.File.Condition(block), neverMatch: current.neverMatch,
	}
	kind, applies := w.visitor.enterBlock(header)
	return blockState{applies: applies && !current.neverMatch, kind: kind, condition: header.condition}
}

// readIncludes は、Include 行ひとつが取り込むファイルを、その行の状態のまま読む。
// state は値で渡るので、戻ったあとの呼び出し元の状態は Include 行の時点のままである。
func (w *loadOrderWalker) readIncludes(current readingFile, lineNumber int, state blockState) {
	for _, edge := range current.node.Includes {
		if edge.Line != lineNumber {
			continue
		}
		for _, match := range edge.Matches {
			w.readFile(match, state, current.neverMatch || !state.applies)
		}
	}
}

// hostBlockApplies は、Host ブロックが alias に効くか、そしてどう一致したかを報告する。
//
// 否定のパターンに当たれば、ほかのパターンに当たっていても効かない。OpenSSH の
// Host 行の規則である。比較は大文字小文字を区別する。Host BASTION だけを持つ設定に
// `ssh -G bastion` を投げると、そのブロックではなく Host * の値が返る。
func hostBlockApplies(block config.Block, alias string) (kind string, applies bool) {
	for _, pattern := range block.Patterns {
		if pattern.Negated && sshmatch.Pattern(pattern.Value, alias) {
			return "", false
		}
	}
	for _, pattern := range block.Patterns {
		if pattern.Negated || !sshmatch.Pattern(pattern.Value, alias) {
			continue
		}
		if pattern.Wildcard {
			return SourceWildcard, true
		}
		return SourceExact, true
	}
	return "", false
}

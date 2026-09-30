package config

import (
	"bytes"
	"errors"
	"strings"
)

// ErrSnapshotIncomplete は、確認画面が示す接続先を、この設定について保証できない
// ことを表す。
//
// internal/effective が OpenSSH と同じ値を出せない形の Include（読めないファイル、
// 循環、深すぎる入れ子、展開できないパターン）を含む設定である。確認と実行の
// 証拠を固定しても、確認画面が OpenSSH なら選ばない接続先を示しうる。まとめた設定が
// MaxSnapshotSize を超えるときも、これを返す。
var ErrSnapshotIncomplete = errors.New("this configuration cannot be fixed as confirmation evidence")

// MaxSnapshotSize は、証拠として固定する設定の上限である。公開鍵のリモート登録は、
// 確認の画面と実行する登録が同じ設定を見ているかを、このまとめた設定のダイジェストで
// 比べる（diagnostics.ConnectionSnapshot）。広い Include グラフをまとめたバイト列を、
// メモリに無制限に積まないためにある。
const MaxSnapshotSize = 4 << 20

// Snapshot は、確認の証拠にする設定グラフを、読み込み順に並べた 1 つのバイト列として
// 固定する。
//
// 公開鍵の登録のように、確認してから実行する操作は、このバイト列のダイジェストを
// 確認の証拠に含める。確認から実行までの間に、到達するどのファイルが変わっても
// 証拠が変わり、確認は無効になる。Include はその行の位置に取り込んだファイルの
// 中身で置き換える。
func Snapshot(graph *Graph) ([]byte, error) {
	if graph == nil || graph.Root == "" {
		return nil, ErrSnapshotIncomplete
	}
	for _, diagnostic := range graph.Diagnostics {
		switch diagnostic.Code {
		case DiagnosticIncludeUnreadable,
			DiagnosticIncludeCycle,
			DiagnosticIncludeDepthExceeded,
			DiagnosticIncludeUnsupported,
			DiagnosticIncludeEmpty:
			return nil, ErrSnapshotIncomplete
		}
	}
	for _, node := range graph.Nodes {
		if node == nil {
			continue
		}
		for _, edge := range node.Includes {
			// OpenSSH は Include 内の ${ENV} を ssh の環境から展開する。Resolver は
			// 同じ値を持たないので、effective はそのファイルを読めず、確認画面の
			// 接続先が OpenSSH の選ぶものと違いうる。
			if strings.Contains(edge.Pattern, "$") {
				return nil, ErrSnapshotIncomplete
			}
		}
	}

	var output bytes.Buffer
	active := make(map[string]bool)
	var inline func(string) error
	inline = func(filePath string) error {
		if active[filePath] {
			return ErrSnapshotIncomplete
		}
		node := graph.Nodes[filePath]
		if node == nil || node.File == nil {
			return ErrSnapshotIncomplete
		}
		active[filePath] = true
		defer delete(active, filePath)

		edgesByLine := make(map[int][]Edge)
		for _, edge := range node.Includes {
			edgesByLine[edge.Line] = append(edgesByLine[edge.Line], edge)
		}
		for index, line := range node.File.Lines {
			if line.Kind != LineDirective || !EqualKeyword(line.Keyword, "Include") {
				output.WriteString(line.Render())
				if output.Len() > MaxSnapshotSize {
					return ErrSnapshotIncomplete
				}
				continue
			}
			edges := edgesByLine[index+1]
			if len(edges) == 0 {
				return ErrSnapshotIncomplete
			}
			for _, edge := range edges {
				if edge.Expanded == "" {
					return ErrSnapshotIncomplete
				}
				for _, match := range edge.Matches {
					before := output.Len()
					if err := inline(match); err != nil {
						return err
					}
					if output.Len() > before && output.Bytes()[output.Len()-1] != '\n' {
						output.WriteByte('\n')
					}
					if output.Len() > MaxSnapshotSize {
						return ErrSnapshotIncomplete
					}
				}
			}
		}
		return nil
	}
	if err := inline(graph.Root); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

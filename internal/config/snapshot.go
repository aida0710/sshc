package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

// ErrSnapshotIncomplete は、OpenSSH があとで別のファイルを読み得る Include を、
// 現在の Graph だけでは不変な設定へ展開できないことを表す。
var ErrSnapshotIncomplete = errors.New("configuration cannot be reduced to one immutable snapshot")

// MaxSnapshotSize bounds the one-file configuration handed from the local
// server to the CLI. Individual source files are already bounded; this keeps a
// wide Include graph from turning one loopback response and temporary file
// into an unbounded allocation.
const MaxSnapshotSize = 4 << 20

// Snapshot は解決済み Include を読み込まれたバイト列へ置き換え、OpenSSH が
// 追加の可変ファイルを開かずに読める単一設定を返す。
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
			DiagnosticIncludeEmpty,
			DiagnosticIncludeConditional:
			return nil, ErrSnapshotIncomplete
		}
	}
	for _, node := range graph.Nodes {
		if node == nil {
			continue
		}
		for _, edge := range node.Includes {
			// OpenSSH は Include 内の環境変数を子プロセスの環境から展開する。
			// Resolver が同じ値を持たないまま空の glob と扱ったものを削除すると、
			// snapshot の意味が元設定と変わるので、安全に固定できない。
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

// Digest commits a short-lived capability to the entire resolved graph,
// including conditions that Snapshot intentionally refuses to flatten. Paths
// and file boundaries are included so moving identical text is still a
// configuration change. This function never evaluates Match or executes SSH.
func Digest(graph *Graph) (string, error) {
	if graph == nil || graph.Root == "" {
		return "", ErrSnapshotIncomplete
	}
	var material bytes.Buffer
	for _, diagnostic := range graph.Diagnostics {
		material.WriteString(diagnostic.Code)
		material.WriteByte(0)
		material.WriteString(diagnostic.Path)
		material.WriteByte(0)
		material.WriteString(strconv.Itoa(diagnostic.Line))
		material.WriteByte(0)
		material.WriteString(diagnostic.Detail)
		material.WriteByte(0)
	}
	for _, path := range graph.Order {
		node := graph.Nodes[path]
		if node == nil || node.File == nil {
			return "", ErrSnapshotIncomplete
		}
		material.WriteString(path)
		material.WriteByte(0)
		for _, line := range node.File.Lines {
			material.WriteString(line.Render())
		}
		material.WriteByte(0)
	}
	sum := sha256.Sum256(material.Bytes())
	return hex.EncodeToString(sum[:]), nil
}

package application

import "sshc/internal/config"

type Visit struct {
	// Path は、その行が属するファイルの絶対 path である。
	Path string
	// Index は、そのファイル内での 0-based の行 index である。
	Index     int
	Line      config.Line
	Block     config.Block
	Condition string
}

func WalkDirectives(graph *config.Graph, visit func(Visit) bool) {
	if graph == nil || graph.Root == "" {
		return
	}
	walkNode(graph, graph.Root, map[string]bool{}, visit)
}

func walkNode(graph *config.Graph, filePath string, chain map[string]bool, visit func(Visit) bool) bool {
	node, ok := graph.Nodes[filePath]
	if !ok || node.File == nil || chain[filePath] {
		return true
	}
	chain[filePath] = true
	defer delete(chain, filePath)

	includedAtLine := make(map[int][]string, len(node.Includes))
	for _, edge := range node.Includes {
		includedAtLine[edge.Line] = append(includedAtLine[edge.Line], edge.Matches...)
	}

	blocks := node.File.Blocks()
	current := 0
	for index := range node.File.Lines {
		for current+1 < len(blocks) && blocks[current+1].Header <= index {
			current++
		}
		line := node.File.Lines[index]
		if line.Kind == config.LineDirective {
			if !visit(Visit{
				Path:      filePath,
				Index:     index,
				Line:      line,
				Block:     blocks[current],
				Condition: node.File.Condition(blocks[current]),
			}) {
				return false
			}
		}
		for _, match := range includedAtLine[index+1] {
			if !walkNode(graph, match, chain, visit) {
				return false
			}
		}
	}
	return true
}

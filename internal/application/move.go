package application

import (
	"errors"

	"sshc/internal/config"
)

var (
	ErrDuplicateDestinationAlias = errors.New("the destination file already declares this alias")
	ErrSameFileMove              = errors.New("source and destination are the same file")
	ErrAliasAlreadyDeclared      = errors.New("another block in the configuration already declares this alias")
)

// ExtractHostBlock は alias を宣言するブロックを取り除き、その行を返す。
func ExtractHostBlock(file *config.File, alias string) ([]config.Line, error) {
	block, ok := FindHostBlock(file, alias)
	if !ok {
		return nil, ErrHostNotFound
	}
	start := file.CommentRun(block.Header)
	end := file.CommentRun(block.End)
	start, end = ClampToRegion(file, start, end)
	extracted := make([]config.Line, 0, end-start)
	extracted = append(extracted, file.Lines[start:end]...)

	remaining := make([]config.Line, 0, len(file.Lines)-len(extracted))
	remaining = append(remaining, file.Lines[:start]...)
	remaining = append(remaining, file.Lines[end:]...)
	file.Lines = remaining
	return extracted, nil
}

func AppendHostBlock(file *config.File, lines []config.Line) {
	if len(lines) == 0 {
		return
	}
	if len(file.Lines) > 0 {
		ending := dominantEnding(file)
		last := &file.Lines[len(file.Lines)-1]
		if last.Ending == "" {
			last.Ending = ending
		}
		if last.Kind != config.LineBlank {
			file.Lines = append(file.Lines, config.Line{Kind: config.LineBlank, Ending: ending})
		}
	}
	file.Lines = append(file.Lines, lines...)
}

// DuplicateHostBlock は、接続に付随するコメントを含めて同じファイルの末尾へ複製し、
// 複製側の concrete alias だけを置き換える。呼び出し側は Include graph 全体で
// newAlias が未使用であることを確認してから呼ぶ。
func DuplicateHostBlock(file *config.File, alias, newAlias string) error {
	copyForExtraction := &config.File{Lines: append([]config.Line(nil), file.Lines...)}
	lines, err := ExtractHostBlock(copyForExtraction, alias)
	if err != nil {
		return err
	}
	duplicate := &config.File{Lines: lines}
	block, ok := FindHostBlock(duplicate, alias)
	if !ok {
		return ErrHostNotFound
	}
	if err := RenameHostAlias(duplicate, block, alias, newAlias); err != nil {
		return err
	}
	AppendHostBlock(file, duplicate.Lines)
	return nil
}

// MoveHostBlock は 1 個のホストブロックを source から destination へ移動する。
func MoveHostBlock(source, destination *config.File, alias string) ([]config.Line, error) {
	if _, exists := FindHostBlock(destination, alias); exists {
		return nil, ErrDuplicateDestinationAlias
	}
	extracted, err := ExtractHostBlock(source, alias)
	if err != nil {
		return nil, err
	}
	AppendHostBlock(destination, extracted)
	return extracted, nil
}

// movedAliases は移動したブロックが宣言する具体的な alias を列挙し、呼び出し側が move
func movedAliases(lines []config.Line) []string {
	block := &config.File{Lines: lines}
	var aliases []string
	for _, candidate := range block.Blocks() {
		if candidate.Kind != config.BlockHost {
			continue
		}
		for _, pattern := range candidate.Patterns {
			if pattern.Negated || pattern.Wildcard {
				continue
			}
			aliases = append(aliases, pattern.Value)
		}
	}
	return aliases
}

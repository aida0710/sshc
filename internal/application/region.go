package application

import (
	"errors"
	"strings"

	"sshc/internal/config"
)

const (
	RegionStartMarker = "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads."
	RegionEndMarker   = "# <<< sshc groups"
	regionNote        = "# Edit through the UI; lines between these markers are replaced on the next save."
)

var (
	// ErrRegionDamaged は、2 個のマーカーのうち片方しか持たない生成領域を報告する。
	ErrRegionDamaged = errors.New("the generated group region has only one of its markers")
	// ErrRegionIncludeAlreadyPresent は、connections tree または generated
	ErrRegionIncludeAlreadyPresent = errors.New("an existing Include already reaches the generated group files")
)

// 生成領域 planner が生成する Notice code である。
const (
	NoticeGroupIncludePresent = "group_include_already_present"
	NoticeRegionDamaged       = "generated_region_damaged"
)

// RegionPlan は、生成領域を設置する編集を記述する。
type RegionPlan struct {
	InsertAt    int
	ReplaceFrom int
	ReplaceTo   int
	Replacing   bool
	RemoveFrom  int
	RemoveTo    int
	Removing    bool
	Lines       []config.Line
}

// GeneratedRegion は FindRegion の結果を config.Resolver 用の範囲へ変換する。
func GeneratedRegion(file *config.File) (start, end int, ok bool) {
	start, end, found, err := FindRegion(file)
	if err != nil || !found {
		return 0, 0, false
	}
	return start, end, true
}

// FindRegion は、マーカーによって生成領域を見つける。
func FindRegion(file *config.File) (start, end int, found bool, err error) {
	start, end = -1, -1
	for index, line := range file.Lines {
		if line.Kind != config.LineComment {
			continue
		}
		switch strings.TrimSpace(line.Text) {
		case RegionStartMarker:
			if start < 0 {
				start = index
			}
		case RegionEndMarker:
			if end < 0 {
				end = index
			}
		}
	}
	switch {
	case start >= 0 && end > start:
		return start, end, true, nil
	case start < 0 && end < 0:
		return -1, -1, false, nil
	default:
		return -1, -1, false, ErrRegionDamaged
	}
}

func ClampToRegion(file *config.File, start, end int) (int, int) {
	regionStart, regionEnd, found, err := FindRegion(file)
	if err != nil || !found {
		return start, end
	}
	if regionStart > start && regionStart < end {
		end = regionStart
	}
	if regionEnd >= start && regionEnd < end {
		start = regionEnd + 1
	}
	return start, end
}

// PlanRegion は、生成領域が何を含み、どこに属するべきかを決める。
func PlanRegion(file *config.File, groups []string, groupsFile string) (RegionPlan, error) {
	start, end, found, err := FindRegion(file)
	if err != nil {
		return RegionPlan{}, err
	}

	ending := dominantEnding(file)
	lines := []config.Line{
		{Kind: config.LineComment, Text: RegionStartMarker, Ending: ending},
		{Kind: config.LineComment, Text: regionNote, Ending: ending},
	}
	for _, pattern := range append(groupPatterns(groups), groupsFile) {
		include, buildErr := buildLine("", "Include", []string{pattern}, ending)
		if buildErr != nil {
			return RegionPlan{}, buildErr
		}
		lines = append(lines, include)
	}
	lines = append(lines, config.Line{Kind: config.LineComment, Text: RegionEndMarker, Ending: ending})

	if found {
		if file.Condition(file.BlockAt(start)) == "" {
			return RegionPlan{ReplaceFrom: start, ReplaceTo: end + 1, Replacing: true, Lines: lines}, nil
		}
		// 生成領域は Host または Match ブロックの内側に座っていて、その Include
		without := withoutLines(file, start, end+1)
		insertAt, positionErr := regionPosition(without, groups, groupsFile)
		if positionErr != nil {
			return RegionPlan{}, positionErr
		}
		return RegionPlan{
			RemoveFrom: start, RemoveTo: end + 1, Removing: true,
			InsertAt: insertAt, Lines: lines,
		}, nil
	}
	insertAt, err := regionPosition(file, groups, groupsFile)
	if err != nil {
		return RegionPlan{}, err
	}
	return RegionPlan{InsertAt: insertAt, Lines: lines}, nil
}

// withoutLines は、1 個の半開区間を取り除いてファイルをコピーする。
func withoutLines(file *config.File, from, to int) *config.File {
	lines := make([]config.Line, 0, len(file.Lines)-(to-from))
	lines = append(lines, file.Lines[:from]...)
	lines = append(lines, file.Lines[to:]...)
	return &config.File{Lines: lines}
}

func groupPatterns(groups []string) []string {
	patterns := make([]string, 0, len(groups))
	for _, group := range groups {
		patterns = append(patterns, GroupIncludePattern(group))
	}
	return patterns
}

// regionPosition は、生成領域がどこに属するかを計算する。最初の Host
func regionPosition(file *config.File, groups []string, groupsFile string) (int, error) {
	if err := checkExistingIncludes(file, groups, groupsFile); err != nil {
		return 0, err
	}
	for index, line := range file.Lines {
		if line.Kind != config.LineDirective {
			continue
		}
		if config.EqualKeyword(line.Keyword, "Host") || config.EqualKeyword(line.Keyword, "Match") {
			return file.CommentRun(index), nil
		}
	}
	return len(file.Lines), nil
}

func checkExistingIncludes(file *config.File, groups []string, groupsFile string) error {
	claimed := make(map[string]bool, len(groups)+1)
	for _, pattern := range append(groupPatterns(groups), groupsFile) {
		claimed[pattern] = true
	}
	for index, line := range file.Lines {
		if line.Kind != config.LineDirective || !config.EqualKeyword(line.Keyword, "Include") {
			continue
		}
		if file.Condition(file.BlockAt(index)) != "" {
			continue
		}
		for _, value := range line.Values() {
			if claimed[value] || strings.HasPrefix(value, ConnectionsDirectory+"/") {
				return ErrRegionIncludeAlreadyPresent
			}
		}
	}
	return nil
}

func ApplyRegion(file *config.File, plan RegionPlan) error {
	if plan.Removing {
		if plan.RemoveFrom < 0 || plan.RemoveTo > len(file.Lines) || plan.RemoveFrom > plan.RemoveTo {
			return ErrEditLineOutsideBlock
		}
		rest := append([]config.Line(nil), file.Lines[plan.RemoveTo:]...)
		file.Lines = append(file.Lines[:plan.RemoveFrom:plan.RemoveFrom], rest...)
	}
	if plan.Replacing {
		if plan.ReplaceFrom < 0 || plan.ReplaceTo > len(file.Lines) || plan.ReplaceFrom > plan.ReplaceTo {
			return ErrEditLineOutsideBlock
		}
		rest := append([]config.Line(nil), file.Lines[plan.ReplaceTo:]...)
		file.Lines = append(append(file.Lines[:plan.ReplaceFrom:plan.ReplaceFrom], plan.Lines...), rest...)
		return nil
	}
	if plan.InsertAt < 0 || plan.InsertAt > len(file.Lines) {
		return ErrEditLineOutsideBlock
	}
	for offset, line := range plan.Lines {
		insertLine(file, plan.InsertAt+offset, line)
	}
	return nil
}

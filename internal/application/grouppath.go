package application

import (
	"sort"
	"sshc/internal/validate"
	"strings"
)

const (
	ConnectionsDirectory = "connections"
	KeysDirectory        = "keys"

	MaxGroupSegments = validate.MaxGroupSegments
)

var ErrInvalidGroupName = validate.ErrInvalidGroupName

// ValidateGroupName は、この表記をグループとして受け付けてよいかを報告する。
func ValidateGroupName(name string) error { return validate.GroupName(name) }

func GroupDirectory(name string) string { return ConnectionsDirectory + "/" + name }

// GroupKeyDirectory は、グループの鍵を保持するワークスペース相対のディレクトリである。
func GroupKeyDirectory(name string) string { return KeysDirectory + "/" + name }

// GroupIncludePattern は、1 つのグループのファイルを読む Include 引数である。
func GroupIncludePattern(name string) string { return GroupDirectory(name) + "/*.conf" }

// groupFileSuffix は GroupIncludePattern が一致させる拡張子である。
const groupFileSuffix = ".conf"

func GroupFileName(base string) string {
	if strings.HasSuffix(base, groupFileSuffix) && base != groupFileSuffix {
		return base
	}
	return base + groupFileSuffix
}

func GroupOfPath(relative string) (name string, inGroup bool) {
	return groupOfPath(relative, ConnectionsDirectory)
}

// GroupOfKeyPath は、鍵ファイルが属するグループを報告する。
func GroupOfKeyPath(relative string) (name string, inGroup bool) {
	return groupOfPath(relative, KeysDirectory)
}

func groupOfPath(relative, root string) (string, bool) {
	cleaned := strings.TrimPrefix(strings.ReplaceAll(relative, "\\", "/"), "./")
	segments := strings.Split(cleaned, "/")
	if len(segments) < 3 || segments[0] != root {
		return "", false
	}
	name := strings.Join(segments[1:len(segments)-1], "/")
	if ValidateGroupName(name) != nil {
		return "", false
	}
	return name, true
}

func ParentGroupName(name string) string {
	index := strings.LastIndex(name, "/")
	if index < 0 {
		return ""
	}
	return name[:index]
}

// GroupSegments は、グループ名をディレクトリ構成要素へ分割する。
func GroupSegments(name string) []string {
	if name == "" {
		return nil
	}
	return strings.Split(name, "/")
}

// GroupDepth は、グループ名中のディレクトリ数を数える。
func GroupDepth(name string) int { return len(GroupSegments(name)) }

func GroupNameOrder(names []string, order map[string]int) []string {
	ordered := append([]string(nil), names...)
	sort.SliceStable(ordered, func(first, second int) bool {
		firstDepth, secondDepth := GroupDepth(ordered[first]), GroupDepth(ordered[second])
		if firstDepth != secondDepth {
			return firstDepth > secondDepth
		}
		if order[ordered[first]] != order[ordered[second]] {
			return order[ordered[first]] < order[ordered[second]]
		}
		return ordered[first] < ordered[second]
	})
	return ordered
}

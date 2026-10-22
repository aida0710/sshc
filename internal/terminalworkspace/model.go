// Package terminalworkspace は、ターミナルのセッションをまとめるワークスペースの定義を
// 持つ。どのセッションがどのワークスペースに入っているかは、セッションと同じく
// エンジンのメモリだけにあり、ここには定義（名前と表示するか）だけを保存する。
package terminalworkspace

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// SchemaVersion は workspaces.json の形の版である。1 と 2 は、画面を分割して
	// 保存していた頃の形で、読まずに無いものとして扱う（分割の機能ごと消したため）。
	SchemaVersion = 3
	// PathRelative は、ワークスペースの定義を置くファイルである。このマシンの
	// エンジンのものなので、同期では運ばない。
	PathRelative = "sshc/workspaces.json"
	// MaxWorkspaces は、持てるワークスペースの数の上限である。サイドバーの木に
	// 並べて選べる数として、分割の頃の保存数の上限と同じ値にした。
	MaxWorkspaces = 50
	// MaxNameRunes は、ワークスペースの名前の長さの上限である。
	MaxNameRunes = 64

	// DefaultID は、画面から開いたセッションが入る、消せないワークスペースである。
	DefaultID = "default"
	// AgentsID は、`sshc terminal create` で開いたセッションが既定で入る、消せない
	// ワークスペースである。LLM などが自由に開いた接続を、既定では表示しない。
	AgentsID = "agents"
)

var (
	ErrNotFound           = errors.New("terminal workspace not found")
	ErrLimit              = errors.New("terminal workspace limit reached")
	ErrInvalidName        = errors.New("terminal workspace name is invalid")
	ErrBuiltIn            = errors.New("a built-in terminal workspace cannot be deleted")
	ErrUnsupportedVersion = errors.New("terminal workspaces were written by a newer version of sshc")
	ErrInvalidDocument    = errors.New("terminal workspaces document is invalid")
)

// Workspace は、ワークスペースひとつの定義である。
type Workspace struct {
	ID string `json:"id"`
	// Name が空なら、画面がその言語の既定の名前を出す。消せないワークスペースだけが
	// 空になりうる。
	Name   string `json:"name"`
	Hidden bool   `json:"hidden"`
}

// BuiltIn は、消せないワークスペースかを返す。
func (w Workspace) BuiltIn() bool { return isBuiltIn(w.ID) }

func isBuiltIn(id string) bool { return id == DefaultID || id == AgentsID }

// builtIns は、ファイルが無いときに使う、消せないワークスペースである。
func builtIns() []Workspace {
	return []Workspace{{ID: DefaultID}, {ID: AgentsID, Hidden: true}}
}

// normalizeName は、前後の空白を除いた名前を返す。消せないワークスペースだけが
// 空の名前（既定の名前に戻す）を持てる。
func normalizeName(name string, builtIn bool) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" && !builtIn {
		return "", ErrInvalidName
	}
	if utf8.RuneCountInString(name) > MaxNameRunes {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

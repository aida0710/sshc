// Package workspace は、端末ローカルに保存する接続集合と分割レイアウトを扱う。
//
// 保存するのは接続 alias と pane tree だけである。terminal session ID や process の
// 生存状態は保存せず、読み直した pane は再接続が必要なものとして扱う。
package workspace

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	validator "sshc/internal/validate"
)

const (
	SchemaVersion = 1
	PathRelative  = "sshc/workspaces.json"

	MaxWorkspaces = 50
	MaxPanes      = 50
	MaxDepth      = 12
	MaxIDRunes    = 64
	MaxNameRunes  = 80

	MinSplitRatio = 10
	MaxSplitRatio = 90
)

var (
	ErrNotFound          = errors.New("no such terminal workspace")
	ErrLimit             = errors.New("the terminal workspace limit has been reached")
	ErrInvalidWorkspace  = errors.New("the terminal workspace is invalid")
	ErrInvalidDocument   = errors.New("the terminal workspace document is invalid")
	ErrUnsupportedSchema = errors.New("terminal workspaces were written by a newer version of sshc")
)

type Direction string

const (
	Horizontal Direction = "horizontal"
	Vertical   Direction = "vertical"
)

func (direction Direction) valid() bool {
	return direction == Horizontal || direction == Vertical
}

// Pane は、レイアウト内の表示場所とそこへ接続する ssh_config alias を表す。
type Pane struct {
	ID    string `json:"id"`
	Alias string `json:"alias"`
}

// Split は、ふたつの子を並べる向きと先頭側の割合を表す。
type Split struct {
	Direction Direction `json:"direction"`
	Ratio     int       `json:"ratio"`
	First     Node      `json:"first"`
	Second    Node      `json:"second"`
}

// Node は pane または split のどちらか一方である。
type Node struct {
	Pane  *Pane  `json:"pane,omitempty"`
	Split *Split `json:"split,omitempty"`
}

// Workspace は、名前を付けて保存された接続集合と分割レイアウトである。
type Workspace struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Layout        Node   `json:"layout"`
	FocusedPaneID string `json:"focusedPaneId"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

// Aliases は、tree の走査順を維持した重複なしの接続 alias を返す。
func (workspace Workspace) Aliases() []string {
	aliases := make([]string, 0)
	seen := map[string]bool{}
	walkPanes(workspace.Layout, func(pane Pane) {
		if !seen[pane.Alias] {
			seen[pane.Alias] = true
			aliases = append(aliases, pane.Alias)
		}
	})
	return aliases
}

func validate(workspace Workspace) error {
	if !validText(workspace.ID, MaxIDRunes) || !validText(workspace.Name, MaxNameRunes) ||
		!validText(workspace.FocusedPaneID, MaxIDRunes) {
		return ErrInvalidWorkspace
	}
	seen := map[string]bool{}
	panes := 0
	if err := validateNode(workspace.Layout, 1, seen, &panes); err != nil {
		return err
	}
	if !seen[workspace.FocusedPaneID] {
		return ErrInvalidWorkspace
	}
	return nil
}

func validateNode(node Node, depth int, seen map[string]bool, panes *int) error {
	if depth > MaxDepth || (node.Pane == nil) == (node.Split == nil) {
		return ErrInvalidWorkspace
	}
	if node.Pane != nil {
		if !validText(node.Pane.ID, MaxIDRunes) || validator.Alias(node.Pane.Alias) != nil || seen[node.Pane.ID] {
			return ErrInvalidWorkspace
		}
		seen[node.Pane.ID] = true
		(*panes)++
		if *panes > MaxPanes {
			return ErrInvalidWorkspace
		}
		return nil
	}
	if !node.Split.Direction.valid() || node.Split.Ratio < MinSplitRatio || node.Split.Ratio > MaxSplitRatio {
		return ErrInvalidWorkspace
	}
	if err := validateNode(node.Split.First, depth+1, seen, panes); err != nil {
		return err
	}
	return validateNode(node.Split.Second, depth+1, seen, panes)
}

func validText(value string, limit int) bool {
	if strings.TrimSpace(value) != value || value == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > limit {
		return false
	}
	for _, character := range value {
		if character == utf8.RuneError || unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func walkPanes(node Node, visit func(Pane)) {
	if node.Pane != nil {
		visit(*node.Pane)
		return
	}
	if node.Split == nil {
		return
	}
	walkPanes(node.Split.First, visit)
	walkPanes(node.Split.Second, visit)
}

func clone(source Workspace) Workspace {
	copy := source
	copy.Layout = cloneNode(source.Layout)
	return copy
}

func cloneNode(source Node) Node {
	var copied Node
	if source.Pane != nil {
		pane := *source.Pane
		copied.Pane = &pane
	}
	if source.Split != nil {
		split := *source.Split
		split.First = cloneNode(source.Split.First)
		split.Second = cloneNode(source.Split.Second)
		copied.Split = &split
	}
	return copied
}

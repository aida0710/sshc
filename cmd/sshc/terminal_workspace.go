package main

import (
	"context"
	"fmt"

	"sshc/internal/api"
	"sshc/internal/terminalworkspace"
)

// resolveTerminalWorkspace は、--workspace の値をワークスペースの ID にする。空なら、
// コマンドで開いたセッションを画面に混ぜないよう、非表示の agents にする。ID と名前の
// どちらでも選べる。
func resolveTerminalWorkspace(ctx context.Context, engine *engineAPI, requested string) (string, error) {
	if requested == "" {
		return terminalworkspace.AgentsID, nil
	}
	var listed api.TerminalWorkspaceList
	if err := engine.getJSON(ctx, "/api/v1/terminal/workspaces", &listed); err != nil {
		return "", err
	}
	for _, workspace := range listed.Workspaces {
		if workspace.Id == requested || (workspace.Name != "" && workspace.Name == requested) {
			return workspace.Id, nil
		}
	}
	return "", fmt.Errorf("no terminal workspace has the ID or name %q", requested)
}

// terminalSessionListing は、terminal list の結果である。JSON は engine の一覧と同じ形で、
// 人が読む表だけがワークスペースを名前で出す。
type terminalSessionListing struct {
	api.TerminalSessionList
	workspaceNames map[string]string
}

// workspaceName は、ワークスペースの名前を返す。名前の無い組み込みのものと、一覧に
// 無いものは ID のまま出す。
func (l terminalSessionListing) workspaceName(id string) string {
	if name := l.workspaceNames[id]; name != "" {
		return name
	}
	return id
}

func listTerminalSessions(ctx context.Context, engine *engineAPI) (terminalSessionListing, error) {
	sessions, err := terminalSessions(ctx, engine)
	if err != nil {
		return terminalSessionListing{}, err
	}
	var workspaces api.TerminalWorkspaceList
	if err := engine.getJSON(ctx, "/api/v1/terminal/workspaces", &workspaces); err != nil {
		return terminalSessionListing{}, err
	}
	names := make(map[string]string, len(workspaces.Workspaces))
	for _, workspace := range workspaces.Workspaces {
		names[workspace.Id] = workspace.Name
	}
	return terminalSessionListing{TerminalSessionList: sessions, workspaceNames: names}, nil
}

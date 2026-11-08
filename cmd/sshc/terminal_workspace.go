package main

import (
	"context"
	"fmt"
	"slices"

	"sshc/internal/api"
	"sshc/internal/terminalworkspace"
)

// builtInWorkspaceDisplayNames は、名前の無い組み込みのワークスペースを画面が出す名前である
// （web/src/i18n/messages の en.ts と ja.ts の workspace.defaultName と workspace.agentsName）。
// 利用者は画面に見える名前で --workspace を書くので、画面の言語によらずどちらの名前でも選べるようにする。
var builtInWorkspaceDisplayNames = map[string][]string{
	terminalworkspace.DefaultID: {"Main", "メイン"},
	terminalworkspace.AgentsID:  {"Agents", "エージェント"},
}

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
	if id, found := findTerminalWorkspace(listed.Workspaces, requested); found {
		return id, nil
	}
	return "", fmt.Errorf("%w: %q", errTerminalWorkspaceNotFound, requested)
}

// findTerminalWorkspace は、ID か利用者が付けた名前が同じワークスペースを探し、無ければ、名前の無い
// 組み込みのワークスペースを画面の名前で探す。利用者が「Main」のような名前を付けたワークスペースは、
// 画面の言語で変わる組み込みの名前より先に選ぶ。名前を付け直した組み込みのものは、画面にもその名前が
// 出るので、付けた名前だけで選ぶ。
func findTerminalWorkspace(workspaces []api.TerminalWorkspace, requested string) (string, bool) {
	for _, workspace := range workspaces {
		if workspace.Id == requested || (workspace.Name != "" && workspace.Name == requested) {
			return workspace.Id, true
		}
	}
	for _, workspace := range workspaces {
		if workspace.Name == "" && slices.Contains(builtInWorkspaceDisplayNames[workspace.Id], requested) {
			return workspace.Id, true
		}
	}
	return "", false
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

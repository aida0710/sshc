package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"sshc/internal/application"
	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/storage"
)

// Connection は、端末から名指しできる接続先ひとつと、それを見分けるための値である。
//
// **ここに載る値は、実際に接続へ使われるものと同じでなければならない。** 一覧が
// 別の解決器を通っていた頃、Match ブロックの下に書かれた HostName は画面に出ず、
// 選んだ先と繋がる先が食い違った。
type Connection struct {
	Alias    string
	HostName string
	User     string
	// Port は既定（22）のときは空である。全部に 22 と書いても何も見分けられない。
	Port string
}

// ReadConnections は、~/.ssh/config と到達できる Include が宣言する具体的な接続先を、
// OpenSSH と同じ読み取り順で返す。
//
// **`Host *` のようなパターンは接続先の名前ではないので返さない。** 同じ名前は
// 最初の一度だけである。
//
// 設定がまだ無いことは空の一覧だが、**あるのに読めないことはエラーである** ——
// 壊れた設定を空の設定に見せると、ホストが無いことにされてしまう。
func ReadConnections(home string) ([]Connection, error) {
	workspace, graph, err := readConfigGraph(home)
	if err != nil {
		return nil, err
	}
	facts := application.LocalFactsFor(workspace.Home())
	listed := make([]Connection, 0)
	for _, alias := range concreteAliases(graph) {
		connection := Connection{Alias: alias, HostName: alias}
		resolution := effective.Resolve(graph, alias, facts)
		if value := resolution.Values.First("hostname"); value != "" {
			connection.HostName = value
		}
		connection.User = resolution.Values.First("user")
		if port := resolution.Values.First("port"); port != "" && port != "22" {
			connection.Port = port
		}
		listed = append(listed, connection)
	}
	return listed, nil
}

// ReadWorkspaceMetadata は、接続先に付いた印（お気に入り・タグ）を返す。
//
// 一覧を出す側は設定と印の両方を要るが、印はワークスペースの持ち物なので、
// 設定の読み取りとは別の入口にしてある。読めなければ印が無いだけで、一覧は出る。
func ReadWorkspaceMetadata(home string) (application.Metadata, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		return application.Metadata{}, err
	}
	metadata, _, err := application.NewMetadataStore(workspace).Load()
	return metadata, err
}

// concreteAliases は、OpenSSH と同じ読み取り順で Host 行を訪れ、実際にコマンドの
// 宛先として使える具体名だけを返す。
func concreteAliases(graph *config.Graph) []string {
	seen := map[string]bool{}
	aliases := []string{}
	application.WalkDirectives(graph, func(visit application.Visit) bool {
		if visit.Block.Kind != config.BlockHost || visit.Block.Header != visit.Index {
			return true
		}
		for _, pattern := range visit.Block.Patterns {
			if pattern.Value == "" || pattern.Negated || pattern.Wildcard || seen[pattern.Value] {
				continue
			}
			seen[pattern.Value] = true
			aliases = append(aliases, pattern.Value)
		}
		return true
	})
	sort.Strings(aliases)
	return aliases
}

// readConfigGraph は、`~/.ssh/config` と到達できる Include を解決する。
func readConfigGraph(home string) (*storage.Workspace, *config.Graph, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		return nil, nil, err
	}
	entry := filepath.Join(workspace.Root(), "config")
	graph, err := storage.NewResolver(workspace).Resolve(entry)
	if err != nil {
		return nil, nil, fmt.Errorf("read config: %w", err)
	}
	// config がまだ存在しないことは空の一覧である。一方、存在するのに読めない場合は、
	// 正しい一覧を返せないので成功したふりをしない。
	if root := graph.Nodes[graph.Root]; root != nil && root.File == nil && !root.Missing {
		return nil, nil, errors.New("cannot read ~/.ssh/config")
	}
	return workspace, graph, nil
}

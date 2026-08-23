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

// Connection は、端末から指定できる接続先ひとつと、それを見分けるための値である。
type Connection struct {
	Alias    string
	HostName string
	User     string
	// Port は既定（22）のときは空である。全部に 22 と書いても何も見分けられない。
	Port string
}

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
func ReadWorkspaceMetadata(home string) (application.Metadata, error) {
	workspace, err := storage.NewWorkspace(storage.OSFileSystem{}, home)
	if err != nil {
		return application.Metadata{}, err
	}
	metadata, _, err := application.NewMetadataStore(workspace).Load()
	return metadata, err
}

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
	if root := graph.Nodes[graph.Root]; root != nil && root.File == nil && !root.Missing {
		return nil, nil, errors.New("cannot read ~/.ssh/config")
	}
	return workspace, graph, nil
}

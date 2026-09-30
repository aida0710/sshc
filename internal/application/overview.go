package application

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"sshc/internal/config"
	"sshc/internal/effective"
	"sshc/internal/storage"
)

// Overview は、Connections tree、Include graph、metadata を構築する。
func (s *Service) Overview() (Overview, error) {
	graph, err := s.resolve()
	if err != nil {
		return Overview{}, err
	}
	root := s.workspace.Root()
	hosts, notices := ProjectHosts(graph, root)
	facts := s.localFacts()
	for index := range hosts {
		alias := hosts[index].Identity.Alias
		if alias == "" {
			continue
		}
		resolution := effective.Resolve(graph, alias, facts)
		if len(resolution.Refusals) != 0 {
			continue
		}
		hosts[index].HostName = resolution.Values.First("hostname")
		hosts[index].User = resolution.Values.First("user")
		hosts[index].Port = resolution.Values.First("port")
	}

	stored, _, err := s.metadata.Load()
	if err != nil {
		return Overview{}, err
	}
	identities := make([]HostIdentity, 0, len(hosts))
	for _, host := range hosts {
		if !host.Identity.IsZero() {
			identities = append(identities, host.Identity)
		}
	}
	reconciled, orphanNotices := ReconcileMetadata(stored, identities)
	// Do not show an old machine's detected icon after SSH settings change.
	for i, host := range reconciled.Hosts {
		if host.DetectedOS == "" {
			continue
		}
		if !s.detectionApplies(graph, host.Identity, host.DetectedOSBinding) {
			reconciled.Hosts[i].DetectedOS = ""
			reconciled.Hosts[i].DetectedOSBinding = ""
		}
	}
	notices = append(notices, orphanNotices...)
	notices = append(notices, s.unreachedConnectionFiles(graph)...)

	entryNode := graph.Nodes[s.entryPath]
	var groups []GroupView
	if entryNode != nil && entryNode.File != nil {
		if _, _, _, regionErr := FindRegion(entryNode.File); errors.Is(regionErr, ErrRegionDamaged) {
			notices = append(notices, Notice{
				Code: NoticeRegionDamaged, Path: s.displayPath(s.entryPath),
			})
		} else {
			present, presentErr := s.presentGroupDirectories()
			if presentErr != nil {
				return Overview{}, presentErr
			}
			var groupNotices []Notice
			groups, groupNotices = BuildGroupsView(entryNode.File, hosts, reconciled, present)
			notices = append(notices, groupNotices...)
		}
	}

	overview := Overview{
		Entry:    NewFileRef(root, s.entryPath),
		Hosts:    hosts,
		Groups:   groups,
		Metadata: reconciled,
		Notices:  notices,
	}
	for _, nodePath := range graph.Order {
		node := graph.Nodes[nodePath]
		reference := NewFileRef(root, nodePath)
		file := FileNode{
			File:     reference,
			Missing:  node.Missing,
			Editable: node.Editable && !reference.External,
			Loads:    node.Loads,
		}
		for _, edge := range node.Includes {
			include := IncludeReference{Line: edge.Line, Pattern: edge.Pattern, Condition: edge.Condition}
			for _, match := range edge.Matches {
				include.Matches = append(include.Matches, NewFileRef(root, match))
			}
			file.Includes = append(file.Includes, include)
		}
		overview.Files = append(overview.Files, file)
	}
	for _, diagnostic := range graph.Diagnostics {
		overview.Diagnostics = append(overview.Diagnostics, NewDiagnosticView(root, diagnostic))
	}
	pending, err := s.Pending()
	if err != nil {
		return Overview{}, err
	}
	overview.Pending = pending

	if overview.Files == nil {
		overview.Files = []FileNode{}
	}
	if overview.Hosts == nil {
		overview.Hosts = []HostEntry{}
	}
	// エントリファイルが無いワークスペースは、ここを nil のまま通る。
	if overview.Groups == nil {
		overview.Groups = []GroupView{}
	}
	if overview.Diagnostics == nil {
		overview.Diagnostics = []DiagnosticView{}
	}
	if overview.Notices == nil {
		overview.Notices = []Notice{}
	}
	return overview, nil
}

// HostDetail は、説明された値と共に 1 個のホストブロックを射影する。
func (s *Service) HostDetail(relative, alias string) (HostDetail, error) {
	graph, err := s.resolve()
	if err != nil {
		return HostDetail{}, err
	}
	identity := HostIdentity{Path: relative, Alias: alias}
	form, err := ProjectHostForm(graph, s.workspace.Root(), identity)
	if err != nil {
		return HostDetail{}, err
	}
	contents, err := s.FileContents(relative)
	if err != nil {
		return HostDetail{}, err
	}
	stored, _, err := s.metadata.Load()
	if err != nil {
		return HostDetail{}, err
	}
	detail := HostDetail{
		Form:      form,
		Effective: ComputeEffective(graph, s.workspace.Root(), alias, s.localFacts()),
		File:      contents,
		Metadata:  HostMetadata{Identity: identity},
	}
	if index := hostMetadataIndex(stored.Hosts, identity); index >= 0 {
		detail.Metadata = stored.Hosts[index]
	}
	return detail, nil
}

// FileContents は、ワークスペース内の設定ファイルを 1 個読む。鍵や sshc の状態ファイルなど、
// 設定として扱えないファイルは ErrNotEditable で断り、中身を返さない。
func (s *Service) FileContents(relative string) (FileContents, error) {
	absolute, err := AbsolutePath(s.workspace.Root(), relative)
	if err != nil {
		return FileContents{}, err
	}
	contents, exists, err := s.readConfigurationFile(absolute)
	if err != nil {
		return FileContents{}, err
	}
	editable := true
	if _, resolveErr := s.workspace.ResolveForWrite(absolute); resolveErr != nil {
		editable = false
	}
	return FileContents{
		File:     NewFileRef(s.workspace.Root(), absolute),
		Contents: string(contents),
		Digest:   storage.Digest(contents),
		Editable: editable,
		Exists:   exists,
	}, nil
}

func (s *Service) presentGroupDirectories() ([]string, error) {
	root := s.workspace.Root()
	base := filepath.Join(root, ConnectionsDirectory)
	var present []string
	var walk func(directory, prefix string, depth int) error
	walk = func(directory, prefix string, depth int) error {
		if depth > MaxGroupSegments {
			return nil
		}
		entries, err := s.workspace.FileSystem().ReadDir(directory)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if prefix != "" {
				name = prefix + "/" + name
			}
			present = append(present, name)
			if err := walk(filepath.Join(directory, entry.Name()), name, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(base, "", 1); err != nil {
		return nil, err
	}
	sort.Strings(present)
	return present, nil
}

func (s *Service) unreachedConnectionFiles(graph *config.Graph) []Notice {
	root := s.workspace.Root()
	base := filepath.Join(root, ConnectionsDirectory)
	var notices []Notice

	var walk func(directory string, depth int)
	walk = func(directory string, depth int) {
		if depth > MaxGroupSegments {
			return
		}
		entries, err := s.workspace.FileSystem().ReadDir(directory)
		if err != nil {
			return
		}
		for _, entry := range entries {
			path := filepath.Join(directory, entry.Name())
			if entry.IsDir() {
				walk(path, depth+1)
				continue
			}
			if !strings.HasSuffix(entry.Name(), groupFileSuffix) {
				continue
			}
			if _, reached := graph.Nodes[path]; reached {
				continue
			}
			notices = appendNotice(notices, Notice{
				Code: NoticeGroupFileUnreached,
				Path: NewFileRef(root, path).Path,
			})
		}
	}
	walk(base, 0)
	return notices
}

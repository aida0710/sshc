package application

import (
	"errors"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"sshc/internal/config"
	"sshc/internal/keys"
	"sshc/internal/storage"
)

var (
	ErrKeyRelocateNotSupported = errors.New("only a private key, or a public key with no private key beside it, can be relocated")
	// ErrKeyRelocateUnchanged は、何も移動しないリクエストを拒否する。
	ErrKeyRelocateUnchanged = errors.New("the key already has that name in that group")
	ErrKeyRelocateBlocked   = errors.New("relocating this key would leave a reference behind")
	ErrKeyReferenceMoved    = errors.New("a configuration file changed while the relocation was being prepared")
)

// Blocker コードは、安定した識別子、':'、詳細の順で構成する。
const (
	BlockerKeyTargetOccupied    = "key_destination_occupied"
	BlockerKeyUnresolved        = "key_reference_unresolved"
	BlockerKeyReferenceExternal = "key_reference_outside_workspace"
	BlockerKeyGroupNotDeclared  = "key_group_not_declared"
	BlockerKeyDestinationRead   = "key_destination_is_config"
	BlockerKeyStateDirectory    = "key_in_state_directory"
)

// KeyRelocateRequest は、鍵の名前、グループ、あるいはその両方を変更する。
type KeyRelocateRequest struct {
	KeyID   string
	NewName *string
	Group   *string
}

// RelocatedKeyFile は relocation が移動した 1 つのファイルをワークスペース相対パスで表す。
type RelocatedKeyFile struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// RewrittenKeyReference は、relocation が更新した 1 つの設定ディレクティブである。
type RewrittenKeyReference struct {
	Directive  string `json:"directive"`
	ConfigPath string `json:"configPath"`
	Line       int    `json:"line"`
	From       string `json:"from"`
	To         string `json:"to"`
}

// KeyRelocateResult は、relocation が行ったこと、あるいはそれを止めたものである。
type KeyRelocateResult struct {
	ID            string                  `json:"id"`
	RelativePath  string                  `json:"relativePath"`
	Group         string                  `json:"group"`
	Files         []RelocatedKeyFile      `json:"files"`
	References    []RewrittenKeyReference `json:"references"`
	Skipped       []string                `json:"skipped"`
	Notes         []string                `json:"notes"`
	Blockers      []string                `json:"blockers"`
	TransactionID string                  `json:"transactionId"`
	Preview       SavePreview             `json:"preview"`
}

func (s *Service) ValidateDeclaredGroup(name string) error {
	if err := ValidateGroupName(name); err != nil {
		return err
	}
	graph, err := s.resolve()
	if err != nil {
		return err
	}
	for _, declared := range s.declaredGroups(graph) {
		if declared == name {
			return nil
		}
	}
	return ErrGroupNotDeclared
}

// keyRelocation は、1 つのファイルについて計画された 1 件の移動である。
type keyRelocation struct {
	from string
	to   string
}

func (s *Service) RelocateKey(inventory *keys.Inventory, request KeyRelocateRequest) (KeyRelocateResult, error) {
	s.saveMutex.Lock()
	defer s.saveMutex.Unlock()

	prepared, result, err := s.planKeyRelocation(inventory, request)
	if err != nil {
		return result, err
	}
	for _, directory := range prepared.directories {
		if err := s.workspace.EnsureDirectory(directory); err != nil {
			return KeyRelocateResult{}, err
		}
	}

	committed, err := s.commitPlannedRequest(prepared, storage.Request{
		Operation: prepared.operation,
		Changes:   prepared.changes,
		Moves:     prepared.moves,
	})
	if err != nil {
		return KeyRelocateResult{}, err
	}
	result.TransactionID = committed.ID
	return result, nil
}

func (s *Service) planKeyRelocation(inventory *keys.Inventory, request KeyRelocateRequest) (planned, KeyRelocateResult, error) {
	graph, err := s.resolve()
	if err != nil {
		return planned{}, KeyRelocateResult{}, err
	}
	item, ok := inventory.Find(request.KeyID)
	if !ok {
		return planned{}, KeyRelocateResult{}, keys.ErrUnknownKey
	}
	members, skipped, err := keyRelocationMembers(inventory, item)
	if err != nil {
		return planned{}, KeyRelocateResult{}, err
	}

	stem := keyStem(item)
	newStem := stem
	if request.NewName != nil {
		newStem = *request.NewName
		if err := keys.ValidateFileName(newStem); err != nil {
			return planned{}, KeyRelocateResult{}, err
		}
	}
	group, _ := GroupOfKeyPath(item.RelativePath)
	newGroup := group
	if request.Group != nil {
		newGroup = *request.Group
		if newGroup != "" {
			if err := ValidateGroupName(newGroup); err != nil {
				return planned{}, KeyRelocateResult{}, err
			}
		}
	}
	if newStem == stem && newGroup == group {
		return planned{}, KeyRelocateResult{}, ErrKeyRelocateUnchanged
	}

	directory := path.Dir(filepath.ToSlash(item.RelativePath))
	if request.Group != nil {
		directory = "."
		if newGroup != "" {
			directory = GroupKeyDirectory(newGroup)
		}
	}
	relocations := make([]keyRelocation, 0, len(members))
	for _, member := range members {
		suffix := strings.TrimPrefix(path.Base(filepath.ToSlash(member.RelativePath)), stem)
		relocations = append(relocations, keyRelocation{
			from: member.RelativePath,
			to:   path.Join(directory, newStem+suffix),
		})
	}

	result := KeyRelocateResult{
		ID:           keys.ItemID(relocations[0].to),
		RelativePath: relocations[0].to,
		Group:        newGroup,
		Files:        make([]RelocatedKeyFile, 0, len(relocations)),
		References:   []RewrittenKeyReference{},
		Skipped:      skipped,
		Notes:        []string{},
		Blockers:     []string{},
	}
	blockers := s.keyRelocationBlockers(graph, inventory, members, relocations, newGroup, request.Group != nil)
	if len(blockers) > 0 {
		result.Blockers = blockers
		return planned{}, result, ErrKeyRelocateBlocked
	}

	prepared := planned{
		operation: "key.relocate",
		base:      map[string][]byte{},
		baseline:  diagnosticBaseline(graph),
		preview:   SavePreview{Operation: "key.relocate"},
	}
	root := s.workspace.Root()
	for _, relocation := range relocations {
		absoluteFrom := filepath.Join(root, filepath.FromSlash(relocation.from))
		contents, readErr := s.workspace.FileSystem().ReadFile(absoluteFrom)
		if readErr != nil {
			return planned{}, KeyRelocateResult{}, readErr
		}
		digest := storage.Digest(contents)
		keys.Wipe(contents)
		prepared.moves = append(prepared.moves, storage.Move{
			From:         absoluteFrom,
			To:           filepath.Join(root, filepath.FromSlash(relocation.to)),
			Precondition: storage.Precondition{Exists: true, Digest: digest},
		})
		result.Files = append(result.Files, RelocatedKeyFile{From: relocation.from, To: relocation.to})
	}
	if directory != "." {
		absolute, dirErr := AbsolutePath(root, directory)
		if dirErr != nil {
			return planned{}, KeyRelocateResult{}, dirErr
		}
		prepared.directories = append(prepared.directories, absolute)
	}

	changes, rewritten, err := s.rewriteKeyReferences(members, relocations)
	if err != nil {
		return planned{}, KeyRelocateResult{}, err
	}
	prepared.changes = changes
	result.References = rewritten
	for _, change := range changes {
		previous, _, readErr := s.readFile(change.Path)
		if readErr != nil {
			return planned{}, KeyRelocateResult{}, readErr
		}
		prepared.base[filepath.Clean(change.Path)] = previous
		prepared.preview.Diffs = append(prepared.preview.Diffs,
			BuildFileDiff(s.displayPath(change.Path), previous, change.Contents))
	}
	result.Preview = prepared.preview
	return prepared, result, nil
}

// keyRelocationMembers は 1 件の relocation が移動するファイルを対象アイテムから順に返す。
func keyRelocationMembers(inventory *keys.Inventory, item *keys.Item) (members []keys.Item, skipped []string, err error) {
	switch item.Kind {
	case keys.KindPrivateKey:
		stem := path.Base(filepath.ToSlash(item.RelativePath))
		directory := path.Dir(filepath.ToSlash(item.RelativePath))
		members = append(members, *item)
		for _, candidate := range inventory.Group(item) {
			if candidate.ID == item.ID {
				continue
			}
			base := path.Base(filepath.ToSlash(candidate.RelativePath))
			if path.Dir(filepath.ToSlash(candidate.RelativePath)) != directory || !strings.HasPrefix(base, stem) {
				skipped = append(skipped, candidate.RelativePath)
				continue
			}
			members = append(members, candidate)
		}
		sort.Strings(skipped)
		return members, skipped, nil
	case keys.KindPublicKey, keys.KindCertificate:
		if privateKeyFor(inventory, item) {
			return nil, nil, ErrKeyRelocateNotSupported
		}
		return []keys.Item{*item}, nil, nil
	default:
		return nil, nil, ErrKeyRelocateNotSupported
	}
}

func privateKeyFor(inventory *keys.Inventory, item *keys.Item) bool {
	fingerprint := item.Fingerprint
	if item.Kind == keys.KindCertificate && item.Certificate != nil {
		fingerprint = item.Certificate.SignedKeyFingerprint
	}
	if fingerprint == "" {
		return false
	}
	for index := range inventory.Items {
		candidate := &inventory.Items[index]
		if candidate.Kind == keys.KindPrivateKey && candidate.Fingerprint == fingerprint {
			return true
		}
	}
	return false
}

// keyStem は、移動するすべてのファイルが共有するベース名の部分である。
func keyStem(item *keys.Item) string {
	base := path.Base(filepath.ToSlash(item.RelativePath))
	if item.Kind == keys.KindPrivateKey {
		return base
	}
	for _, suffix := range []string{"-cert.pub", ".pub"} {
		if trimmed := strings.TrimSuffix(base, suffix); trimmed != base && trimmed != "" {
			return trimmed
		}
	}
	return base
}

func (s *Service) keyRelocationBlockers(
	graph *config.Graph,
	inventory *keys.Inventory,
	members []keys.Item,
	relocations []keyRelocation,
	group string,
	groupRequested bool,
) []string {
	blockers := make([]string, 0)
	root := s.workspace.Root()

	if groupRequested && group != "" {
		declared := false
		for _, name := range s.declaredGroups(graph) {
			if name == group {
				declared = true
				break
			}
		}
		if !declared {
			blockers = append(blockers, BlockerKeyGroupNotDeclared+":"+group)
		}
	}

	for _, relocation := range relocations {
		absolute := filepath.Join(root, filepath.FromSlash(relocation.to))
		if _, err := s.workspace.FileSystem().Lstat(absolute); err == nil {
			blockers = append(blockers, BlockerKeyTargetOccupied+":"+relocation.to)
		}
		if strings.HasPrefix(relocation.to+"/", keys.StateDirectoryName+"/") ||
			strings.HasPrefix(relocation.from+"/", keys.StateDirectoryName+"/") {
			// ごみ箱、バックアップ、ジャーナルはエンジンの状態である。
			blockers = append(blockers, BlockerKeyStateDirectory+":"+relocation.to)
		}
		if reachedByInclude(graph, absolute) {
			blockers = append(blockers, BlockerKeyDestinationRead+":"+relocation.to)
		}
	}

	for _, member := range members {
		for _, reference := range member.References {
			if !s.workspace.Contains(reference.ConfigPath) {
				blockers = append(blockers, BlockerKeyReferenceExternal+":"+s.displayPath(reference.ConfigPath))
			}
		}
	}
	for _, unresolved := range inventory.UnresolvedReferences {
		if risk := unresolvedKeyRisk(unresolved, members); risk != "" {
			blockers = append(blockers, BlockerKeyUnresolved+":"+risk)
		}
	}
	if len(blockers) == 0 {
		return nil
	}
	sort.Strings(blockers)
	return blockers
}

func reachedByInclude(graph *config.Graph, absolute string) bool {
	cleaned := filepath.Clean(absolute)
	for _, node := range graph.Nodes {
		for _, edge := range node.Includes {
			if edge.Expanded == "" {
				continue
			}
			matched, err := filepath.Match(edge.Expanded, cleaned)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

func unresolvedKeyRisk(unresolved keys.UnresolvedReference, members []keys.Item) string {
	if unresolved.Directive == "IdentityAgent" {
		return ""
	}
	switch unresolved.Reason {
	case keys.ReasonOutsideWorkspace:
		return ""
	case keys.ReasonRelativePath:
		base := path.Base(filepath.ToSlash(unresolved.Value))
		for _, member := range members {
			if path.Base(filepath.ToSlash(member.RelativePath)) == base {
				return unresolved.Value
			}
		}
		return ""
	default:
		return unresolved.Value
	}
}

func (s *Service) rewriteKeyReferences(members []keys.Item, relocations []keyRelocation) ([]storage.Change, []RewrittenKeyReference, error) {
	root := s.workspace.Root()
	destinations := make(map[string]string, len(relocations))
	for _, relocation := range relocations {
		destinations[relocation.from] = relocation.to
	}
	byConfigPath := make(map[string][]keys.Reference)
	order := make([]string, 0)
	for _, member := range members {
		for _, reference := range member.References {
			if _, seen := byConfigPath[reference.ConfigPath]; !seen {
				order = append(order, reference.ConfigPath)
			}
			byConfigPath[reference.ConfigPath] = append(byConfigPath[reference.ConfigPath], reference)
		}
	}
	sort.Strings(order)

	changes := make([]storage.Change, 0, len(order))
	rewritten := make([]RewrittenKeyReference, 0)
	for _, configPath := range order {
		contents, err := s.workspace.FileSystem().ReadFile(configPath)
		if err != nil {
			return nil, nil, err
		}
		parsed := config.Parse(contents)
		touched := false

		for _, reference := range byConfigPath[configPath] {
			index := reference.Line - 1
			if index < 0 || index >= len(parsed.Lines) || parsed.Lines[index].Kind != config.LineDirective {
				return nil, nil, ErrKeyReferenceMoved
			}
			line := &parsed.Lines[index]
			for argumentIndex := range line.Arguments {
				argument := &line.Arguments[argumentIndex]
				if strings.HasPrefix(argument.Raw, "#") {
					break
				}
				from, moved := relocationFor(destinations, root, s.workspace, argument.Value)
				if !moved {
					continue
				}
				replacement := rewriteKeyValue(argument.Value, from, destinations[from], root)
				rendered, renderErr := config.RenderArgument(argument.Lead, replacement)
				if renderErr != nil {
					return nil, nil, renderErr
				}
				rewritten = append(rewritten, RewrittenKeyReference{
					Directive:  line.Keyword,
					ConfigPath: s.displayPath(configPath),
					Line:       reference.Line,
					From:       argument.Value,
					To:         replacement,
				})
				*argument = rendered
				touched = true
			}
		}
		if !touched {
			continue
		}
		changes = append(changes, storage.Change{
			Path:         configPath,
			Contents:     parsed.Render(),
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(contents)},
		})
	}
	return changes, rewritten, nil
}

// relocationFor は、ディレクティブの引数がどの移動先ファイルを指定しているかを報告する。
func relocationFor(destinations map[string]string, root string, workspace *storage.Workspace, value string) (string, bool) {
	for from := range destinations {
		if keys.ExpandsTo(workspace, value, filepath.Join(root, filepath.FromSlash(from))) {
			return from, true
		}
	}
	return "", false
}

func rewriteKeyValue(value, from, to, root string) string {
	if prefix, ok := strings.CutSuffix(filepath.ToSlash(value), from); ok {
		return prefix + to
	}
	return filepath.ToSlash(filepath.Join(root, filepath.FromSlash(to)))
}

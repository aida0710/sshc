package sftp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"path"
	"sort"
	"strings"
)

type chmodObservation struct {
	Path     string
	Revision string
}

type chmodTarget struct {
	chmodObservation
	mode fs.FileMode
}

// ChmodPlan owns the remote and, when prepared by TransferManager, path locks.
// Its revision covers the selection, options, ancestors and complete bounded tree.
type ChmodPlan struct {
	Revision        string
	Files           int
	Directories     int
	SkippedSymlinks int
	request         ChmodRequest
	remote          Remote
	chmodRemote     ChmodRemote
	targets         []chmodTarget
	observed        map[string]string
	unlock          func()
	attempted       bool
}

func (p *ChmodPlan) Close() {
	if p.remote != nil {
		_ = p.remote.Close()
		p.remote = nil
	}
	if p.unlock != nil {
		p.unlock()
		p.unlock = nil
	}
}

func normalizeChmodRequest(request ChmodRequest) (ChmodRequest, error) {
	if err := validateAlias(request.Alias); err != nil {
		return ChmodRequest{}, err
	}
	if len(request.Entries) == 0 || len(request.Entries) > MaxChmodSelection ||
		request.Options.FileMode != request.Options.FileMode.Perm() || request.Options.DirectoryMode != request.Options.DirectoryMode.Perm() {
		return ChmodRequest{}, fs.ErrInvalid
	}
	request.Entries = append([]ChmodEntry(nil), request.Entries...)
	for index, entry := range request.Entries {
		if entry.ExpectedRevision == "" {
			return ChmodRequest{}, ErrRevisionRequired
		}
		cleaned, err := cleanMetadataPath(entry.Path)
		if err != nil {
			return ChmodRequest{}, err
		}
		request.Entries[index].Path = cleaned
	}
	sort.Slice(request.Entries, func(left, right int) bool { return request.Entries[left].Path < request.Entries[right].Path })
	for index := 1; index < len(request.Entries); index++ {
		if request.Entries[index-1].Path == request.Entries[index].Path {
			return ChmodRequest{}, fs.ErrInvalid
		}
	}
	return request, nil
}

func (s Service) PrepareChmod(ctx context.Context, request ChmodRequest) (*ChmodPlan, error) {
	normalized, err := normalizeChmodRequest(request)
	if err != nil {
		return nil, err
	}
	remote, err := s.openRequest(ctx, normalized.Alias)
	if err != nil {
		return nil, err
	}
	plan := &ChmodPlan{request: normalized, remote: remote}
	chmodRemote, supported := optionalRemote(remote).(ChmodRemote)
	if !supported {
		plan.Close()
		return nil, ErrUnsupportedOperation
	}
	plan.chmodRemote = chmodRemote
	if err := chmodRemote.CheckChmodNoFollow(); err != nil {
		plan.Close()
		return nil, err
	}
	if err := plan.collect(ctx); err != nil {
		plan.Close()
		return nil, err
	}
	return plan, nil
}

// Preparation and waiting live in the domain; handlers only authorize this plan.
func (m *TransferManager) PrepareChmod(ctx context.Context, request ChmodRequest) (*ChmodPlan, error) {
	if m == nil || m.Service == nil || m.isClosed() {
		return nil, ErrUnavailable
	}
	plan, err := m.Service.PrepareChmod(ctx, request)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(plan.targets))
	for index, target := range plan.targets {
		paths[index] = target.Path
	}
	plan.unlock, err = m.LockOperationContext(ctx, request.Alias, paths...)
	if err == nil {
		err = plan.verify(ctx)
	}
	if err != nil {
		plan.Close()
		return nil, err
	}
	return plan, nil
}

func (p *ChmodPlan) inspect(ctx context.Context, candidate string) (fs.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := p.remote.Lstat(candidate)
	if err != nil {
		return nil, err
	}
	if !metadataTypeKnown(info) {
		return nil, ErrMetadataUnavailable
	}
	revision := metadataRevision(info)
	if previous, exists := p.observed[candidate]; exists && previous != revision {
		return nil, ErrConflict
	}
	p.observed[candidate] = revision
	if len(p.observed) > maxChmodPlanEntries {
		return nil, ErrTraversalLimit
	}
	return info, nil
}

func (p *ChmodPlan) inspectAncestors(ctx context.Context, candidate string) error {
	ancestors := []string{}
	for directory := path.Dir(candidate); ; directory = path.Dir(directory) {
		ancestors = append(ancestors, directory)
		if directory == "/" {
			break
		}
	}
	for index := len(ancestors) - 1; index >= 0; index-- {
		info, err := p.inspect(ctx, ancestors[index])
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return ErrNotDirectory
		}
	}
	return nil
}

func (p *ChmodPlan) collect(ctx context.Context) error {
	p.observed = make(map[string]string)
	selected := make(map[string]fs.FileInfo, len(p.request.Entries))
	for _, entry := range p.request.Entries {
		if err := p.inspectAncestors(ctx, entry.Path); err != nil {
			return err
		}
		info, err := p.inspect(ctx, entry.Path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() && !info.IsDir() {
			return ErrNotRegularFile
		}
		if metadataRevision(info) != entry.ExpectedRevision {
			return ErrConflict
		}
		selected[entry.Path] = info
	}
	for _, entry := range p.request.Entries {
		// An explicitly selected descendant is still validated, but changes once.
		if p.request.Options.Recursive && hasSelectedAncestor(entry.Path, selected) {
			continue
		}
		p.addTarget(entry.Path, selected[entry.Path])
		if p.request.Options.Recursive && selected[entry.Path].IsDir() {
			if err := p.collectTree(ctx, entry.Path); err != nil {
				return err
			}
		}
	}
	sort.Slice(p.targets, func(left, right int) bool {
		leftDepth, rightDepth := strings.Count(p.targets[left].Path, "/"), strings.Count(p.targets[right].Path, "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		return p.targets[left].Path < p.targets[right].Path
	})
	observations := make([]chmodObservation, 0, len(p.observed))
	for candidate, revision := range p.observed {
		observations = append(observations, chmodObservation{Path: candidate, Revision: revision})
	}
	sort.Slice(observations, func(left, right int) bool { return observations[left].Path < observations[right].Path })
	encoded, err := json.Marshal(struct {
		Request      ChmodRequest
		Observations []chmodObservation
	}{p.request, observations})
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	p.Revision = chmodRevisionPrefix + hex.EncodeToString(digest[:])
	return nil
}

func hasSelectedAncestor(candidate string, selected map[string]fs.FileInfo) bool {
	for directory := path.Dir(candidate); directory != "/"; directory = path.Dir(directory) {
		if _, exists := selected[directory]; exists {
			return true
		}
	}
	return false
}

func (p *ChmodPlan) addTarget(candidate string, info fs.FileInfo) {
	mode := p.request.Options.FileMode
	if info.IsDir() {
		mode = p.request.Options.DirectoryMode
		p.Directories++
	} else {
		p.Files++
	}
	p.targets = append(p.targets, chmodTarget{chmodObservation: chmodObservation{Path: candidate, Revision: metadataRevision(info)}, mode: mode})
}

func (p *ChmodPlan) collectTree(ctx context.Context, root string) error {
	return walkBoundedTree(ctx, p.remote, boundedTreeWalk{
		root: root,
		verifyDirectory: func(directory string) error {
			info, err := p.inspect(ctx, directory)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return ErrConflict
			}
			return nil
		},
		visit: func(directory string, child fs.FileInfo) error {
			candidate := path.Join(directory, child.Name())
			info, err := p.inspect(ctx, candidate)
			if err != nil {
				return err
			}
			if metadataRevision(info) != metadataRevision(child) {
				return ErrConflict
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				p.SkippedSymlinks++
				return nil
			}
			if !info.Mode().IsRegular() && !info.IsDir() {
				return ErrUnsupportedEntry
			}
			p.addTarget(candidate, info)
			return nil
		},
	})
}

func (p *ChmodPlan) verify(ctx context.Context) error {
	current := &ChmodPlan{remote: p.remote, request: p.request}
	if err := current.collect(ctx); err != nil {
		return err
	}
	if current.Revision != p.Revision {
		return ErrConflict
	}
	return nil
}

func (p *ChmodPlan) Apply(ctx context.Context, expectedRevision string) (ChmodResult, error) {
	result := ChmodResult{Items: len(p.targets)}
	if p.remote == nil || p.attempted {
		return result, ErrUnavailable
	}
	if expectedRevision == "" {
		return result, ErrRevisionRequired
	}
	if expectedRevision != p.Revision {
		return result, ErrConflict
	}
	// A changed late target or added child must refuse the entire selection.
	if err := p.verify(ctx); err != nil {
		return result, err
	}
	p.attempted = true
	for _, target := range p.targets {
		if err := p.inspectAncestors(ctx, target.Path); err != nil {
			return result, err
		}
		info, err := p.inspect(ctx, target.Path)
		if err != nil {
			return result, err
		}
		if metadataRevision(info) != target.Revision {
			return result, ErrConflict
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		// A lost status response can follow an applied server mutation, even on
		// the first item. Callers must report that outcome as potentially partial.
		result.Started = true
		if err := p.chmodRemote.ChmodNoFollow(target.Path, target.mode); err != nil {
			return result, err
		}
		result.Applied++
	}
	return result, nil
}

func (p *ChmodPlan) Entry(candidate string) (Entry, error) {
	cleaned, err := cleanMetadataPath(candidate)
	if err != nil {
		return Entry{}, err
	}
	info, err := p.remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	return entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)}), nil
}

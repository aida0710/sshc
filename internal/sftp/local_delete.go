package sftp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
)

// Keep selection size bounded independently of the recursive tree budget.
const maxLocalDeleteSelection = 200

type LocalDeleteEntry struct {
	Path             string
	ExpectedRevision string
}

type localDeleteTarget struct {
	parent     *os.Root
	name       string
	absolute   string
	metadata   fs.FileInfo
	linkTarget string
	identity   string
}

// LocalDeletePlan owns pinned directories and the mutation/admission lock.
// Callers must Close it, including after a confirmation is refused.
type LocalDeletePlan struct {
	Revision          string
	Items             int64
	targets           []localDeleteTarget
	roots             []*os.Root
	selectedPaths     []string
	directoryChildren map[*os.Root][]fs.FileInfo
	homeMetadata      fs.FileInfo
	unlock            func()
}

func (p *LocalDeletePlan) Close() {
	for index := len(p.roots) - 1; index >= 0; index-- {
		_ = p.roots[index].Close()
	}
	p.roots = nil
	if p.unlock != nil {
		p.unlock()
		p.unlock = nil
	}
}

// PrepareLocalDelete validates every selected tree before confirmation. The
// digest binds the token to descendants as well as the selected root entries.
func (m *TransferManager) PrepareLocalDelete(ctx context.Context, selection []LocalDeleteEntry) (*LocalDeletePlan, error) {
	if m == nil || m.isClosed() {
		return nil, ErrUnavailable
	}
	if len(selection) == 0 || len(selection) > maxLocalDeleteSelection {
		return nil, ErrInvalidPath
	}
	m.localMutationsMutex.Lock()
	plan := &LocalDeletePlan{unlock: m.localMutationsMutex.Unlock, directoryChildren: make(map[*os.Root][]fs.FileInfo)}
	if err := plan.collect(ctx, selection); err != nil {
		plan.Close()
		return nil, err
	}
	if err := m.refuseLocalTransferOverlap(plan.selectedPaths); err != nil {
		plan.Close()
		return nil, err
	}
	if err := plan.verify(ctx); err != nil {
		plan.Close()
		return nil, err
	}
	digest := sha256.New()
	for _, target := range plan.targets {
		_, _ = fmt.Fprintf(digest, "%q\x00%s\x00%s\x00%q\x00", target.absolute, target.identity, metadataRevision(target.metadata), target.linkTarget)
	}
	plan.Revision = "local-delete-sha256:" + hex.EncodeToString(digest.Sum(nil))
	plan.Items = int64(len(plan.targets))
	return plan, nil
}

func (p *LocalDeletePlan) collect(ctx context.Context, selection []LocalDeleteEntry) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	p.homeMetadata, err = os.Stat(home)
	if err != nil {
		return err
	}
	selected := append([]LocalDeleteEntry(nil), selection...)
	sort.Slice(selected, func(left, right int) bool { return selected[left].Path < selected[right].Path })
	for _, entry := range selected {
		if entry.ExpectedRevision == "" {
			return ErrRevisionRequired
		}
		target, err := openLocalMutationTarget(entry.Path)
		if err != nil {
			return err
		}
		p.roots = append(p.roots, target.parent)
		for _, previous := range p.selectedPaths {
			if localPathsOverlap(previous, target.absolute) {
				return ErrInvalidPath
			}
		}
		p.selectedPaths = append(p.selectedPaths, target.absolute)
		metadata, err := target.parent.Lstat(target.name)
		if err != nil {
			return err
		}
		if metadataRevision(metadata) != entry.ExpectedRevision {
			return ErrConflict
		}
		if err := p.collectTree(ctx, localDeleteTarget{parent: target.parent, name: target.name, absolute: target.absolute, metadata: metadata}, 0); err != nil {
			return err
		}
	}
	return nil
}

func (p *LocalDeletePlan) collectTree(ctx context.Context, target localDeleteTarget, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > maxDeleteDepth || len(p.targets) >= maxDeleteEntries {
		return ErrTraversalLimit
	}
	if isLocalTemporaryName(target.name) {
		return ErrConflict
	}
	identity, err := localDeletionIdentity(target.parent, target.name, target.metadata)
	if err != nil {
		return err
	}
	target.identity = identity
	if !target.metadata.IsDir() {
		if target.metadata.Mode()&fs.ModeSymlink != 0 {
			linkTarget, err := target.parent.Readlink(target.name)
			if err != nil {
				return err
			}
			target.linkTarget = linkTarget
		} else if !target.metadata.Mode().IsRegular() {
			return ErrUnsupportedEntry
		}
		p.targets = append(p.targets, target)
		return nil
	}
	if os.SameFile(target.metadata, p.homeMetadata) {
		return ErrRootOperation
	}
	directory, err := target.parent.OpenRoot(target.name)
	if err != nil {
		return err
	}
	p.roots = append(p.roots, directory)
	opened, err := directory.Lstat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(opened, target.metadata) {
		return ErrConflict
	}
	children, err := readLocalMutationChildren(directory)
	if err != nil {
		return err
	}
	p.directoryChildren[directory] = children
	for _, child := range children {
		childTarget := localDeleteTarget{parent: directory, name: child.Name(), absolute: joinLocalMutationPath(target.absolute, child.Name()), metadata: child}
		if err := p.collectTree(ctx, childTarget, depth+1); err != nil {
			return err
		}
	}
	p.targets = append(p.targets, target)
	if len(p.targets) > maxDeleteEntries {
		return ErrTraversalLimit
	}
	return nil
}

func readLocalMutationChildren(directory *os.Root) ([]fs.FileInfo, error) {
	file, err := directory.Open(".")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// One extra entry proves the traversal budget has already been exceeded,
	// without allocating an unbounded listing of a very large directory.
	children, err := file.Readdir(maxDeleteEntries + 1)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(children) > maxDeleteEntries {
		return nil, ErrTraversalLimit
	}
	for _, child := range children {
		if isLocalTemporaryName(child.Name()) {
			return nil, ErrConflict
		}
		if !validLocalMutationName(child.Name()) {
			return nil, ErrInvalidPath
		}
	}
	sort.Slice(children, func(left, right int) bool { return children[left].Name() < children[right].Name() })
	return children, nil
}

func (p *LocalDeletePlan) verify(ctx context.Context) error {
	for _, target := range p.targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := target.parent.Lstat(target.name)
		if err != nil {
			return ErrConflict
		}
		if !os.SameFile(current, target.metadata) || metadataRevision(current) != metadataRevision(target.metadata) {
			return ErrConflict
		}
		identity, err := localDeletionIdentity(target.parent, target.name, current)
		if err != nil || identity != target.identity {
			return ErrConflict
		}
		if target.metadata.Mode()&fs.ModeSymlink != 0 {
			linkTarget, err := target.parent.Readlink(target.name)
			if err != nil || linkTarget != target.linkTarget {
				return ErrConflict
			}
		}
	}
	for directory, expectedChildren := range p.directoryChildren {
		if err := ctx.Err(); err != nil {
			return err
		}
		children, err := readLocalMutationChildren(directory)
		if err != nil {
			return err
		}
		if len(children) != len(expectedChildren) {
			return ErrConflict
		}
		for index, child := range children {
			if child.Name() != expectedChildren[index].Name() {
				return ErrConflict
			}
		}
	}
	return nil
}

func (p *LocalDeletePlan) Delete(ctx context.Context, expectedRevision string) error {
	if p.unlock == nil {
		return ErrUnavailable
	}
	if expectedRevision == "" {
		return ErrRevisionRequired
	}
	if expectedRevision != p.Revision {
		return ErrConflict
	}
	// A failed check changes nothing, including another selected tree.
	if err := p.verify(ctx); err != nil {
		return err
	}
	for _, target := range p.targets {
		if err := ctx.Err(); err != nil {
			return err
		}
		// The parent is pinned, and the name has no separator. A directory
		// replaced by a link cannot redirect removal of its children.
		if err := target.parent.Remove(target.name); err != nil {
			return err
		}
	}
	return nil
}

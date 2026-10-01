package sftp

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"sort"
	"time"
)

// maxComparedEntries bounds how many entries a directory comparison reads from
// both sides together.
const maxComparedEntries = 20_000

// maxTransferTreeEntries bounds the entries a remote copy, move or get walks in
// one folder tree. Planning and the transfer itself count against the same
// limit, so a source that grows in between cannot keep a job running without
// end. It is separate from maxComparedEntries so that changing what a
// comparison may read does not change what a transfer may copy.
const maxTransferTreeEntries = 20_000

// CompareDirectories compares metadata without downloading file contents.
// Symlinks are listed but never followed.
func (s Service) CompareDirectories(ctx context.Context, leftAlias, leftPath, rightAlias, rightPath string) (DirectoryComparison, error) {
	leftRoot, err := cleanPublicPath(leftPath, true)
	if err != nil {
		return DirectoryComparison{}, err
	}
	rightRoot, err := cleanPublicPath(rightPath, true)
	if err != nil {
		return DirectoryComparison{}, err
	}
	left, err := s.readTree(ctx, leftAlias, leftRoot)
	if err != nil {
		return DirectoryComparison{}, err
	}
	right, err := s.readTree(ctx, rightAlias, rightRoot)
	if err != nil {
		return DirectoryComparison{}, err
	}
	keys := make([]string, 0, len(left)+len(right))
	seen := make(map[string]struct{}, len(left)+len(right))
	for candidate := range left {
		seen[candidate] = struct{}{}
		keys = append(keys, candidate)
	}
	for candidate := range right {
		if _, ok := seen[candidate]; ok {
			continue
		}
		keys = append(keys, candidate)
	}
	if len(keys) > maxComparedEntries {
		return DirectoryComparison{}, ErrCompareLimit
	}
	sort.Strings(keys)
	result := DirectoryComparison{LeftPath: leftRoot, RightPath: rightRoot, Entries: make([]DirectoryDifference, 0, len(keys))}
	entryIndex := make(map[string]int, len(keys))
	for _, relative := range keys {
		leftEntry, leftOK := left[relative]
		rightEntry, rightOK := right[relative]
		difference := DirectoryDifference{RelativePath: relative}
		if leftOK {
			copy := leftEntry
			difference.Left = &copy
		}
		if rightOK {
			copy := rightEntry
			difference.Right = &copy
		}
		switch {
		case !rightOK:
			difference.Status = DirectoryLeftOnly
		case !leftOK:
			difference.Status = DirectoryRightOnly
		case leftEntry.Type != rightEntry.Type:
			difference.Status = DirectoryTypeMismatch
		case leftEntry.Type == EntryDirectory:
			difference.Status = DirectorySame
		case leftEntry.Size == rightEntry.Size && leftEntry.Mode.Perm() == rightEntry.Mode.Perm() && leftEntry.ModifiedAt.Equal(rightEntry.ModifiedAt):
			difference.Status = DirectorySame
		default:
			difference.Status = DirectoryDifferent
		}
		entryIndex[relative] = len(result.Entries)
		result.Entries = append(result.Entries, difference)
	}
	// A directory is different when any descendant differs. This lets the UI
	// summarize a large tree without pretending equal directory mtimes imply
	// equal contents.
	for index := len(result.Entries) - 1; index >= 0; index-- {
		item := result.Entries[index]
		if item.Status == DirectorySame || item.RelativePath == "" {
			continue
		}
		parent := path.Dir(item.RelativePath)
		for parent != "." && parent != "/" {
			if parentIndex, ok := entryIndex[parent]; ok {
				candidate := &result.Entries[parentIndex]
				if candidate.Status == DirectorySame {
					candidate.Status = DirectoryDifferent
				}
			}
			parent = path.Dir(parent)
		}
	}
	return result, nil
}

func (s Service) readTree(ctx context.Context, alias, root string) (map[string]Entry, error) {
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return nil, err
	}
	defer remote.Close()
	result := make(map[string]Entry)
	pending := []string{root}
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		directory := pending[0]
		pending = pending[1:]
		infos, err := readChildren(ctx, remote, directory)
		if err != nil {
			return nil, err
		}
		for _, info := range infos {
			if isInternalName(info.Name()) {
				continue
			}
			entry := entryFrom(directory, info)
			relative := entry.Path[len(root):]
			if len(relative) > 0 && relative[0] == '/' {
				relative = relative[1:]
			}
			result[relative] = entry
			if len(result) > maxComparedEntries {
				return nil, ErrCompareLimit
			}
			if entry.Type == EntryDirectory {
				pending = append(pending, entry.Path)
			}
		}
	}
	return result, nil
}

func (s Service) PlanRemoteTransfer(ctx context.Context, request RemoteTransferRequest) (RemoteTransferPlan, error) {
	source, target, err := cleanRemoteTransferRequest(request)
	if err != nil {
		return RemoteTransferPlan{}, err
	}
	if request.SourceAlias == request.TargetAlias && source == target {
		return RemoteTransferPlan{}, ErrAlreadyExists
	}
	remote, err := s.openRequest(ctx, request.SourceAlias)
	if err != nil {
		return RemoteTransferPlan{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(source)
	if err != nil {
		return RemoteTransferPlan{}, err
	}
	plan := RemoteTransferPlan{Name: path.Base(source), TotalBytes: info.Size(), Kind: TransferFile}
	if info.IsDir() {
		plan.Kind = TransferFolder
		plan.TotalBytes, err = treeBytes(ctx, remote, source)
	} else if !info.Mode().IsRegular() {
		return RemoteTransferPlan{}, ErrUnsupportedEntry
	}
	_ = target
	return plan, err
}

func treeBytes(ctx context.Context, remote Remote, root string) (int64, error) {
	var total int64
	pending := []string{root}
	visited := 0
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		directory := pending[0]
		pending = pending[1:]
		infos, err := readChildren(ctx, remote, directory)
		if err != nil {
			return 0, err
		}
		for _, info := range infos {
			if isInternalName(info.Name()) {
				continue
			}
			visited++
			if visited > maxTransferTreeEntries {
				return 0, ErrTraversalLimit
			}
			if info.IsDir() {
				pending = append(pending, path.Join(directory, info.Name()))
				continue
			}
			if !info.Mode().IsRegular() {
				return 0, ErrUnsupportedEntry
			}
			total += info.Size()
		}
	}
	return total, nil
}

// CopyRemote streams bytes from source SFTP to target SFTP. It never creates a
// plaintext local file. Each target file is published by atomic sibling rename.
func (s Service) CopyRemote(ctx context.Context, request RemoteTransferRequest, progress func(int64) error) error {
	sourcePath, targetPath, err := cleanRemoteTransferRequest(request)
	if err != nil {
		return err
	}
	if request.SourceAlias == request.TargetAlias && sourcePath == targetPath {
		return ErrAlreadyExists
	}
	source, err := s.openRequest(ctx, request.SourceAlias)
	if err != nil {
		return err
	}
	defer source.Close()
	// Within one host both ends share the connection. A host that answers a
	// one-time code refuses a second login in the same window.
	target := source
	if request.TargetAlias != request.SourceAlias {
		target, err = s.openRequest(ctx, request.TargetAlias)
		if err != nil {
			return err
		}
		defer target.Close()
	}
	info, err := source.Lstat(sourcePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := s.refuseTargetInsideSource(request, source, target, sourcePath, targetPath); err != nil {
			return err
		}
	}
	if request.SourceAlias == request.TargetAlias && request.Operation == RemoteMove {
		move := &renameMove{remote: target, overwrite: request.Overwrite, published: newPublishedNames()}
		return move.move(ctx, sourcePath, targetPath, info)
	}
	if request.Operation == RemoteMove {
		// A move must be all-or-nothing from the user's perspective. Reject trees
		// containing entries that the copier deliberately does not follow before
		// any target data is written.
		if err := validateMovableTree(ctx, source, sourcePath); err != nil {
			return err
		}
	}
	copier := &remoteCopy{
		service: s, source: source, target: target, targetAlias: request.TargetAlias,
		overwrite: request.Overwrite, progress: progress, published: newPublishedNames(),
	}
	if request.Operation == RemoteMove {
		copier.copied = make(map[string]copiedFile)
	}
	if info.IsDir() {
		err = copier.copyDirectory(ctx, sourcePath, targetPath, info.Mode())
	} else if info.Mode().IsRegular() {
		err = copier.copyFile(ctx, sourcePath, targetPath, info)
	} else {
		err = ErrUnsupportedEntry
	}
	if err != nil {
		return err
	}
	if request.Operation == RemoteMove {
		return copier.removeCopiedTree(ctx, sourcePath)
	}
	return nil
}

// renameMove は同じ接続先の中の move を rename で行う。承認された上書きで転送先に
// 同名のフォルダがあるとき、フォルダを丸ごと rename することはできない（OpenSSH の
// sftp-server は既存の移動先への rename を拒む）。そこで中へ降り、無い子は rename、
// 既存のファイルは置換、既存のフォルダはさらに中へ降りて統合する。別の接続先への
// move が copy で既存のフォルダへ統合するのと同じ結果になり、データは engine を通らない。
type renameMove struct {
	remote    Remote
	overwrite bool
	// published は、統合で一度置いた名前を、同じ実行の別の名前で上書きしないための記録。
	// 同じ接続先でも、source と転送先が大文字小文字の扱いの違う mount にありうる。
	published *publishedNames
}

func (m *renameMove) move(ctx context.Context, sourcePath, targetPath string, info fs.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	targetInfo, err := m.remote.Lstat(targetPath)
	if errors.Is(err, fs.ErrNotExist) {
		if err := m.remote.Rename(sourcePath, targetPath); err != nil {
			return err
		}
		m.published.record(targetPath)
		return nil
	}
	if err != nil {
		return err
	}
	if err := m.published.existingEntryError(targetPath, m.overwrite); err != nil {
		return err
	}
	if !info.IsDir() {
		if err := m.remote.Replace(sourcePath, targetPath); err != nil {
			return err
		}
		m.published.record(targetPath)
		return nil
	}
	if !targetInfo.IsDir() {
		return differentKindError(targetPath)
	}
	m.published.record(targetPath)
	children, err := readChildren(ctx, m.remote, sourcePath)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := m.move(ctx, path.Join(sourcePath, child.Name()), path.Join(targetPath, child.Name()), child); err != nil {
			return err
		}
	}
	return m.remote.RemoveDirectory(sourcePath)
}

func cleanRemoteTransferRequest(request RemoteTransferRequest) (string, string, error) {
	if err := validateAlias(request.SourceAlias); err != nil {
		return "", "", err
	}
	if err := validateAlias(request.TargetAlias); err != nil {
		return "", "", err
	}
	if request.Operation != RemoteCopy && request.Operation != RemoteMove {
		return "", "", ErrInvalidTransfer
	}
	source, err := cleanPublicPath(request.SourcePath, false)
	if err != nil {
		return "", "", err
	}
	target, err := cleanPublicPath(request.TargetPath, false)
	if err != nil {
		return "", "", err
	}
	return source, target, nil
}

// remoteCopy は 1 回の remote→remote copy／move を実行する。source が返す名前や
// listing は信用せず、entry 数を plan と同じ予算で数え、copy 済み file の revision を
// 覚えて move の削除前に照合する。
type remoteCopy struct {
	service     Service
	source      Remote
	target      Remote
	targetAlias string
	overwrite   bool
	progress    func(int64) error
	// published は、上書きの承認を、この実行より前から target にあった entry だけに
	// 効かせるための記録。
	published *publishedNames

	transferred int64
	visited     int
	// copied は move のときだけ持つ。copy 済み regular file の source path ごとに、
	// source と target の revision を覚える。move はどちらも一致した file だけを削除する。
	copied map[string]copiedFile
}

// copiedFile は、move が source の file を消してよいかを決める材料。
type copiedFile struct {
	// sourceRevision は copy 直後に確認した source の metadata revision。
	sourceRevision string
	target         string
	// targetRevision は公開した直後の target の metadata revision。削除の直前に
	// target が別の entry に置き換わっていれば、source が唯一の複製になる。
	targetRevision string
}

func (c *remoteCopy) report(delta int64) error {
	c.transferred += delta
	if c.progress == nil {
		return nil
	}
	return c.progress(c.transferred)
}

// visit は plan（treeBytes）と同じ予算で entry を数える。plan と copy の間に source が
// listing を差し替えても、copy が際限なく続くことはない。
func (c *remoteCopy) visit() error {
	c.visited++
	if c.visited > maxTransferTreeEntries {
		return ErrTraversalLimit
	}
	return nil
}

func (c *remoteCopy) copyDirectory(ctx context.Context, sourcePath, targetPath string, mode fs.FileMode) error {
	if err := c.visit(); err != nil {
		return err
	}
	created := false
	if targetInfo, err := c.target.Lstat(targetPath); err == nil {
		if err := c.published.existingEntryError(targetPath, c.overwrite); err != nil {
			return err
		}
		if !targetInfo.IsDir() {
			return differentKindError(targetPath)
		}
	} else if errors.Is(err, fs.ErrNotExist) {
		if err := c.target.Mkdir(targetPath); err != nil {
			return err
		}
		created = true
	} else {
		return err
	}
	c.published.record(targetPath)
	infos, err := readChildren(ctx, c.source, sourcePath)
	if err != nil {
		return err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	for _, info := range infos {
		if err := ctx.Err(); err != nil {
			return err
		}
		if isInternalName(info.Name()) {
			continue
		}
		sourceChild := path.Join(sourcePath, info.Name())
		targetChild := path.Join(targetPath, info.Name())
		switch {
		case info.IsDir():
			if err := c.copyDirectory(ctx, sourceChild, targetChild, info.Mode()); err != nil {
				return err
			}
		case info.Mode().IsRegular():
			if err := c.copyFile(ctx, sourceChild, targetChild, info); err != nil {
				return err
			}
		default:
			return ErrUnsupportedEntry
		}
	}
	if !created {
		return nil
	}
	// The source mode is applied only after the children are written: a folder
	// without owner write (a Go module cache, the Nix store) would refuse them.
	// A copy that stops earlier leaves the folder writable, so it can be rerun.
	return c.target.Chmod(targetPath, mode.Perm())
}

func (c *remoteCopy) copyFile(ctx context.Context, sourcePath, targetPath string, before fs.FileInfo) (resultErr error) {
	if err := c.visit(); err != nil {
		return err
	}
	if _, err := c.target.Lstat(targetPath); err == nil {
		if err := c.published.existingEntryError(targetPath, c.overwrite); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	input, err := c.source.Open(sourcePath)
	if err != nil {
		return err
	}
	defer input.Close()
	temporary, err := c.service.temporaryPath(targetPath)
	if err != nil {
		return err
	}
	output, err := c.target.Create(temporary)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if closeErr := output.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
		if cleanup {
			unpublished := unpublishedFile{service: c.service, alias: c.targetAlias, remote: c.target, path: temporary}
			resultErr = errors.Join(resultErr, unpublished.remove(ctx))
		}
	}()
	written, err := copyContext(ctx, &progressWriter{Writer: output, report: c.report}, input, before.Size())
	if err != nil {
		return err
	}
	if written != before.Size() {
		return ErrConflict
	}
	if err := output.Close(); err != nil {
		return err
	}
	output = closedWriter{}
	if err := c.target.Chmod(temporary, before.Mode().Perm()); err != nil {
		return err
	}
	after, err := c.source.Lstat(sourcePath)
	if err != nil || metadataRevision(before) != metadataRevision(after) {
		return ErrConflict
	}
	if c.overwrite {
		err = c.target.Replace(temporary, targetPath)
	} else {
		err = c.target.Rename(temporary, targetPath)
	}
	if err != nil {
		return err
	}
	cleanup = false
	c.published.record(targetPath)
	// The source's time is set only after publishing: until then the
	// temporary must keep the time of its last write, or a delete or move
	// of the folder would take it for an abandoned one (isAbandonedTemporary).
	if err := c.target.Chtimes(targetPath, before.ModTime()); err != nil {
		return err
	}
	if c.copied == nil {
		return nil
	}
	published, err := c.target.Lstat(targetPath)
	if err != nil {
		return err
	}
	c.copied[sourcePath] = copiedFile{
		sourceRevision: metadataRevision(after),
		target:         targetPath,
		targetRevision: metadataRevision(published),
	}
	return nil
}

// removeCopiedTree は move の後半として source を消す。各 file は削除の直前に、source と
// 公開した target の両方を copy 時の revision と照合する。copy 後に変わった file、copy
// していない entry、target で別の entry に置き換わった file があれば ErrConflict で止まる。
// 止まるまでに消した file は target に同じ内容が公開済みである。
func (c *remoteCopy) removeCopiedTree(ctx context.Context, sourcePath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := c.source.Lstat(sourcePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return ErrConflict
		}
		copied, ok := c.copied[sourcePath]
		if !ok || copied.sourceRevision != metadataRevision(info) {
			return ErrConflict
		}
		published, err := c.target.Lstat(copied.target)
		if err != nil || metadataRevision(published) != copied.targetRevision {
			return ErrConflict
		}
		return c.source.Remove(sourcePath)
	}
	infos, err := readChildren(ctx, c.source, sourcePath)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, child := range infos {
		if isAbandonedTemporary(child, now) {
			if err := c.source.Remove(path.Join(sourcePath, child.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			continue
		}
		if isInternalName(child.Name()) {
			return ErrConflict
		}
		if err := c.removeCopiedTree(ctx, path.Join(sourcePath, child.Name())); err != nil {
			return err
		}
	}
	return c.source.RemoveDirectory(sourcePath)
}

type progressWriter struct {
	Writer interface{ Write([]byte) (int, error) }
	report func(int64) error
}

func (writer *progressWriter) Write(contents []byte) (int, error) {
	written, err := writer.Writer.Write(contents)
	if err == nil && writer.report != nil && written > 0 {
		err = writer.report(int64(written))
	}
	return written, err
}

func validateMovableTree(ctx context.Context, remote Remote, root string) error {
	pending := []string{root}
	visited := 0
	now := time.Now()
	for len(pending) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := pending[0]
		pending = pending[1:]
		info, err := remote.Lstat(current)
		if err != nil {
			return err
		}
		visited++
		if visited > maxTransferTreeEntries {
			return ErrTraversalLimit
		}
		if info.Mode().IsRegular() {
			continue
		}
		if !info.IsDir() {
			return ErrUnsupportedEntry
		}
		children, err := readChildren(ctx, remote, current)
		if err != nil {
			return err
		}
		for _, child := range children {
			if isAbandonedTemporary(child, now) {
				// Not copied, and removed with the source afterwards.
				continue
			}
			if isInternalName(child.Name()) {
				return ErrConflict
			}
			pending = append(pending, path.Join(current, child.Name()))
		}
	}
	return nil
}

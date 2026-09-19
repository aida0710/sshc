package sftp

import (
	"errors"
	"io/fs"
	"path"
	"sync"
)

// Symlinks are shown as what they point to when a person opens them, while
// recursive operations (copy, move, delete, archive, search) keep treating
// them as links so that a loop or an escape from the tree stays impossible.

// A link chain longer than this is treated as a loop.
const maxLinkHops = 16

// Listing a directory of symlinks costs two round trips per link; this many
// links are resolved at the same time.
const linkResolveParallelism = 8

var ErrLinkLoop = errors.New("symlink chain is too long")

// followLink walks a symlink chain from remotePath and returns the path and
// metadata of the entry at its end. A path that is not a symlink is returned
// as is. Relative targets are resolved against the directory of the link, as
// the server would resolve them.
func followLink(remote Remote, remotePath string) (string, fs.FileInfo, error) {
	current := remotePath
	for hop := 0; hop <= maxLinkHops; hop++ {
		info, err := remote.Lstat(current)
		if err != nil {
			return "", nil, err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			return current, info, nil
		}
		target, err := remote.ReadLink(current)
		if err != nil {
			return "", nil, err
		}
		if !path.IsAbs(target) {
			target = path.Join(path.Dir(current), target)
		}
		current = path.Clean(target)
	}
	return "", nil, ErrLinkLoop
}

// fileLocation is a file a person opened by one path while its bytes live at
// another: `shown` is the path they named and the one their pane keeps
// showing; `stored` is where a symlink chain ends and the bytes are read
// from and written to. Both are the same path for anything but a symlink.
type fileLocation struct {
	shown  string
	stored string
}

// locateFile resolves the path a single-file read or write acts on. The
// resolved path is held to the same rules as a path named directly.
func locateFile(remote Remote, cleaned string) (fileLocation, error) {
	resolved, _, err := followLink(remote, cleaned)
	if err != nil {
		return fileLocation{}, err
	}
	if resolved == cleaned {
		return fileLocation{shown: cleaned, stored: cleaned}, nil
	}
	stored, err := cleanPublicPath(resolved, false)
	if err != nil {
		return fileLocation{}, err
	}
	return fileLocation{shown: cleaned, stored: stored}, nil
}

func linkTargetTypeOf(info fs.FileInfo) LinkTargetType {
	switch {
	case info.IsDir():
		return LinkTargetDirectory
	case info.Mode().IsRegular():
		return LinkTargetFile
	}
	return LinkTargetOther
}

// describeLinks fills in, for every symlink in entries, where it points and
// what kind of entry that is. A link whose target cannot be read keeps an
// empty TargetType, which the UI shows as a broken link.
func describeLinks(remote Remote, entries []Entry) {
	var pending sync.WaitGroup
	slots := make(chan struct{}, linkResolveParallelism)
	for index := range entries {
		if entries[index].Type != EntrySymlink {
			continue
		}
		pending.Add(1)
		slots <- struct{}{}
		go func(entry *Entry) {
			defer pending.Done()
			defer func() { <-slots }()
			if target, err := remote.ReadLink(entry.Path); err == nil {
				entry.LinkTarget = target
			}
			_, info, err := followLink(remote, entry.Path)
			if err != nil {
				return
			}
			entry.TargetType = linkTargetTypeOf(info)
			// A link to a file transfers as that file, so the size and time a
			// person sees, and a copy is given, are the file's own.
			if info.Mode().IsRegular() {
				entry.Size = info.Size()
				entry.ModifiedAt = info.ModTime().UTC()
			}
		}(&entries[index])
	}
	pending.Wait()
}

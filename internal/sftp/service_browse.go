package sftp

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
)

// Read-only views of a remote tree: listings, stat, bounded search and
// the directory totals shown before a large copy.

func (s Service) ListDirectory(ctx context.Context, alias, remotePath string) (Listing, error) {
	var cleaned string
	var err error
	if remotePath != "" {
		cleaned, err = cleanPublicPath(remotePath, true)
		if err != nil {
			return Listing{}, err
		}
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Listing{}, err
	}
	defer remote.Close()
	if remotePath == "" {
		workingDirectory, err := remote.Getwd(ctx)
		if err != nil {
			return Listing{}, err
		}
		cleaned, err = cleanPublicPath(workingDirectory, true)
		if err != nil {
			return Listing{}, err
		}
	}

	infos, err := readChildren(ctx, remote, cleaned)
	if err != nil {
		return Listing{}, err
	}
	entries := make([]Entry, 0, len(infos))
	for _, info := range infos {
		if isInternalName(info.Name()) {
			continue
		}
		entries = append(entries, entryFrom(cleaned, info))
	}
	describeLinks(remote, entries)
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].opensAsDirectory() && !entries[right].opensAsDirectory() {
			return true
		}
		if !entries[left].opensAsDirectory() && entries[right].opensAsDirectory() {
			return false
		}
		leftName, rightName := strings.ToLower(entries[left].Name), strings.ToLower(entries[right].Name)
		if leftName == rightName {
			return entries[left].Name < entries[right].Name
		}
		return leftName < rightName
	})
	return Listing{Path: cleaned, Entries: entries}, nil
}

// Search は、あるディレクトリ配下から名前に query を含む項目を集める。
//
// symlink は辿らない。辿れば輪に入りうるし、同じ実体を別の名前で二度返す。
func (s Service) Search(ctx context.Context, alias, remotePath, query string) (SearchResult, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" || len(needle) > MaxSearchQueryBytes {
		return SearchResult{}, ErrInvalidQuery
	}
	root, err := cleanPublicPath(remotePath, true)
	if err != nil {
		return SearchResult{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return SearchResult{}, err
	}
	defer remote.Close()

	result := SearchResult{Path: root, Query: query}
	visited := 0
	pending := []string{root}
	for depth := 0; depth <= maxSearchDepth && len(pending) > 0; depth++ {
		var next []string
		for _, directory := range pending {
			if err := ctx.Err(); err != nil {
				return SearchResult{}, err
			}
			infos, err := readChildren(ctx, remote, directory)
			if err != nil {
				// 読めない枝は飛ばす。権限のない一つのディレクトリで検索
				// 全体を落とすほうが、利用者にとって役に立たない。
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return SearchResult{}, err
				}
				result.Truncated = true
				continue
			}
			for _, info := range infos {
				if isInternalName(info.Name()) {
					continue
				}
				visited++
				if visited > maxSearchVisited {
					result.Truncated = true
					return result, nil
				}
				entry := entryFrom(directory, info)
				if strings.Contains(strings.ToLower(entry.Name), needle) {
					if len(result.Entries) >= maxSearchResults {
						result.Truncated = true
						return result, nil
					}
					result.Entries = append(result.Entries, entry)
				}
				if entry.Type == EntryDirectory {
					next = append(next, entry.Path)
				}
			}
		}
		if depth == maxSearchDepth && len(next) > 0 {
			result.Truncated = true
			break
		}
		pending = next
	}
	return result, nil
}

// DirectoryStats totals regular files below a directory without following
// symlinks. It shares the search traversal budget so a properties dialog can
// never start an unbounded walk of a remote filesystem.
func (s Service) DirectoryStats(ctx context.Context, alias, remotePath string) (DirectoryStats, error) {
	root, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return DirectoryStats{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return DirectoryStats{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(root)
	if err != nil {
		return DirectoryStats{}, err
	}
	if !info.IsDir() {
		return DirectoryStats{}, ErrNotDirectory
	}

	result := DirectoryStats{Path: root, Directories: 1}
	visited := 0
	pending := []string{root}
	for depth := 0; depth <= maxSearchDepth && len(pending) > 0; depth++ {
		var next []string
		for _, directory := range pending {
			if err := ctx.Err(); err != nil {
				return DirectoryStats{}, err
			}
			infos, err := readChildren(ctx, remote, directory)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return DirectoryStats{}, err
				}
				result.Truncated = true
				continue
			}
			for _, child := range infos {
				if isInternalName(child.Name()) {
					continue
				}
				visited++
				if visited > maxSearchVisited {
					result.Truncated = true
					return result, nil
				}
				switch {
				case child.Mode().IsRegular():
					result.Files++
					if child.Size() >= 0 && result.Bytes <= int64(^uint64(0)>>1)-child.Size() {
						result.Bytes += child.Size()
					} else {
						result.Truncated = true
					}
				case child.IsDir():
					result.Directories++
					next = append(next, path.Join(directory, child.Name()))
				}
			}
		}
		if depth == maxSearchDepth && len(next) > 0 {
			result.Truncated = true
			break
		}
		pending = next
	}
	return result, nil
}

func (s Service) List(ctx context.Context, alias, remotePath string) ([]Entry, error) {
	listing, err := s.ListDirectory(ctx, alias, remotePath)
	return listing.Entries, err
}

func (s Service) Stat(ctx context.Context, alias, remotePath string) (Entry, error) {
	cleaned, err := cleanPublicPath(remotePath, true)
	if err != nil {
		return Entry{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Entry{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return Entry{}, err
	}
	entries := []Entry{entryFrom(path.Dir(cleaned), namedInfo{FileInfo: info, name: path.Base(cleaned)})}
	describeLinks(remote, entries)
	return entries[0], nil
}

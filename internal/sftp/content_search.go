package sftp

import (
	"bytes"
	"context"
	"errors"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// Match the editor limit so each search result can be opened for editing.
	maxContentSearchFileBytes = MaxEditableFileBytes
	// Bound SFTP traffic across all files in one interactive search.
	maxContentSearchBytes = 64 << 20
	// Keep long lines from flooding the result list; retain context on each side.
	maxSearchSnippetRunes     = 160
	searchSnippetContextRunes = 40
)

type contentSearch struct {
	remote       Remote
	searchResult SearchResult
	budget       contentReadBudget
	omissions    map[string]int
	visited      int
}

func (s Service) searchContents(ctx context.Context, options SearchOptions) (SearchResult, error) {
	root, err := cleanPublicPath(options.Path, true)
	if err != nil {
		return SearchResult{}, err
	}
	remote, err := s.openRequest(ctx, options.Alias)
	if err != nil {
		return SearchResult{}, err
	}
	defer remote.Close()
	search := contentSearch{
		remote: remote, budget: contentReadBudget{maxBytes: maxContentSearchBytes},
		searchResult: SearchResult{Path: root, Query: options.Query, Mode: SearchContent, Entries: []Entry{}, Matches: []ContentMatch{}},
		omissions:    make(map[string]int),
	}
	err = search.walk(ctx, root)
	search.searchResult.BytesRead = search.budget.bytesRead
	// Stable ordering makes the summary predictable even when no matches exist.
	for _, reason := range []string{omissionSymlink, omissionUnsupported, omissionBinary, omissionFileSize, omissionUnreadable, omissionChanged, omissionByteLimit, omissionResultLimit, omissionEntryLimit, omissionDepthLimit} {
		if count := search.omissions[reason]; count > 0 {
			search.searchResult.Omissions = append(search.searchResult.Omissions, SearchOmission{Reason: reason, Count: count})
		}
	}
	return search.searchResult, err
}

func (search *contentSearch) omit(reason string) {
	search.omissions[reason]++
	search.searchResult.Truncated = true
}

func (search *contentSearch) walk(ctx context.Context, root string) error {
	pending := []string{root}
	for depth := 0; len(pending) > 0; depth++ {
		if depth > maxSearchDepth {
			search.omit(omissionDepthLimit)
			return nil
		}
		var next []string
		for _, directory := range pending {
			children, err := search.visitDirectory(ctx, directory)
			if errors.Is(err, errStopWalk) {
				return nil
			}
			if err != nil {
				return err
			}
			next = append(next, children...)
		}
		pending = next
	}
	return nil
}

func (search *contentSearch) visitDirectory(ctx context.Context, directory string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	children, err := readStableRemoteDirectory(ctx, search.remote, directory)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if directory == search.searchResult.Path || errors.Is(err, ErrInvalidPath) {
			return nil, err
		}
		reason := omissionUnreadable
		if errors.Is(err, ErrConflict) || errors.Is(err, ErrUnsupportedEntry) {
			reason = omissionChanged
		}
		search.omit(reason)
		return nil, nil
	}
	var subdirectories []string
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		search.visited++
		if search.visited > maxSearchVisited {
			search.omit(omissionEntryLimit)
			return nil, errStopWalk
		}
		if isInternalName(child.Name()) {
			continue
		}
		entry := entryFrom(directory, child)
		if entry.Type == EntryDirectory {
			subdirectories = append(subdirectories, path.Join(directory, child.Name()))
			continue
		}
		if err := search.searchFile(ctx, entry); err != nil {
			return nil, err
		}
	}
	return subdirectories, nil
}

func (search *contentSearch) searchFile(ctx context.Context, entry Entry) error {
	if entry.Type != EntryFile {
		if entry.Type == EntrySymlink {
			search.omit(omissionSymlink)
		} else {
			search.omit(omissionUnsupported)
		}
		return nil
	}
	if entry.Size < 0 || entry.Size > maxContentSearchFileBytes {
		search.omit(omissionFileSize)
		return nil
	}
	if entry.Size > search.budget.maxBytes-search.budget.bytesRead {
		search.omit(omissionByteLimit)
		return errStopWalk
	}
	file, err := openStableRemoteFile(search.remote, entry)
	if err != nil {
		return search.skipFile(ctx, err)
	}
	defer file.reader.Close()
	var contents bytes.Buffer
	if err := search.budget.stream(ctx, contentStream{file: file, destination: &contents}); err != nil {
		return search.skipFile(ctx, err)
	}
	if !validText(contents.Bytes()) {
		search.omit(omissionBinary)
		return nil
	}
	return search.matchLines(ctx, entry, contents.String())
}

func (search *contentSearch) skipFile(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, ErrConflict) {
		search.omit(omissionChanged)
	} else {
		search.omit(omissionUnreadable)
	}
	return nil
}

func (search *contentSearch) matchLines(ctx context.Context, entry Entry, contents string) error {
	// Monaco treats LF, CRLF and CR as line endings. Search must use the same
	// numbering before it asks the editor to jump to a matching line.
	contents = strings.ReplaceAll(strings.ReplaceAll(contents, "\r\n", "\n"), "\r", "\n")
	lineNumber := 0
	for line := range strings.SplitSeq(contents, "\n") {
		if err := ctx.Err(); err != nil {
			return err
		}
		lineNumber++
		index := strings.Index(line, search.searchResult.Query)
		if index < 0 {
			continue
		}
		if len(search.searchResult.Matches) >= maxSearchResults {
			search.omit(omissionResultLimit)
			return errStopWalk
		}
		search.searchResult.Matches = append(search.searchResult.Matches, ContentMatch{Entry: entry, Line: lineNumber, Snippet: searchSnippet(line, index)})
	}
	return nil
}

func searchSnippet(line string, matchByte int) string {
	runes := []rune(line)
	matchRune := utf8.RuneCountInString(line[:matchByte])
	start := max(0, matchRune-searchSnippetContextRunes)
	end := min(len(runes), start+maxSearchSnippetRunes)
	snippet := strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, string(runes[start:end]))
	if start > 0 {
		snippet = "…" + snippet
	}
	if end < len(runes) {
		snippet += "…"
	}
	return snippet
}

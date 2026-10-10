package sftp

import (
	"bytes"
	"context"
	"path"
)

type TextReadOptions struct {
	Alias string
	Path  string
	// Search entries carry metadata revisions. An empty revision preserves
	// the editor's ordinary reads, including explicitly opened links.
	ExpectedRevision string
}

// A search result must not resolve a new link or jump to a replaced file.
// Ordinary editor opens retain their established link-following behavior.
func (s Service) ReadTextWithRevision(ctx context.Context, options TextReadOptions) (TextFile, error) {
	if options.ExpectedRevision == "" {
		return s.ReadText(ctx, options.Alias, options.Path)
	}
	cleaned, err := cleanPublicPath(options.Path, false)
	if err != nil {
		return TextFile{}, err
	}
	remote, err := s.openRequest(ctx, options.Alias)
	if err != nil {
		return TextFile{}, err
	}
	defer remote.Close()
	info, err := remote.Lstat(cleaned)
	if err != nil {
		return TextFile{}, existingContentReadError(err)
	}
	entry := entryFrom(path.Dir(cleaned), info)
	if entry.Type != EntryFile || entry.Revision != options.ExpectedRevision {
		return TextFile{}, ErrConflict
	}
	if entry.Size < 0 || entry.Size > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	file, err := openStableRemoteFile(remote, entry)
	if err != nil {
		return TextFile{}, err
	}
	defer file.reader.Close()
	var contents bytes.Buffer
	budget := contentReadBudget{maxBytes: MaxEditableFileBytes}
	if err := budget.stream(ctx, contentStream{file: file, destination: &contents}); err != nil {
		return TextFile{}, err
	}
	if !validText(contents.Bytes()) {
		return TextFile{}, ErrNotUTF8
	}
	return TextFile{Entry: entry, Contents: contents.String(), Revision: contentRevision(contents.Bytes())}, nil
}

package sftp

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
)

// Text and preview reads for the built-in editor, and the write-back that
// refuses to clobber a file edited elsewhere since it was read.

func (s Service) ReadText(ctx context.Context, alias, remotePath string) (TextFile, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return TextFile{}, err
	}
	defer remote.Close()
	file, err := locateFile(remote, cleaned)
	if err != nil {
		return TextFile{}, err
	}
	return readText(ctx, remote, file)
}

// ReadPreview は、詳細モーダルが描ける画像を丸ごと返す。
//
// 型が分かるまでに読むのは先頭 previewSniffBytes だけである。preview に
// できない大きな書庫を、断ると分かっている間ずっと転送し続けない。
func (s Service) ReadPreview(ctx context.Context, alias, remotePath string) (Preview, error) {
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return Preview{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return Preview{}, err
	}
	defer remote.Close()
	file, err := locateFile(remote, cleaned)
	if err != nil {
		return Preview{}, err
	}
	before, err := remote.Lstat(file.stored)
	if err != nil {
		return Preview{}, err
	}
	if !before.Mode().IsRegular() {
		return Preview{}, ErrNotRegularFile
	}
	if before.Size() > MaxPreviewFileBytes {
		return Preview{}, ErrPreviewTooLarge
	}
	contents, contentType, err := readPreviewBytes(ctx, remote, file.stored)
	if err != nil {
		return Preview{}, err
	}
	after, err := remote.Lstat(file.stored)
	if err != nil {
		return Preview{}, err
	}
	// 読んでいるあいだに置き換わったものを、古い metadata のまま見せない。
	if metadataRevision(before) != metadataRevision(after) {
		return Preview{}, ErrConflict
	}
	entry := entryFrom(path.Dir(file.shown), namedInfo{FileInfo: after, name: path.Base(file.shown)})
	return Preview{Entry: entry, ContentType: contentType, Contents: contents, Revision: contentRevision(contents)}, nil
}

func readPreviewBytes(ctx context.Context, remote Remote, cleaned string) ([]byte, string, error) {
	file, err := remote.Open(cleaned)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = file.Close() }()
	reader := &contextReader{ctx: ctx, reader: file}
	head := make([]byte, previewSniffBytes)
	read, err := io.ReadFull(reader, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, "", err
	}
	head = head[:read]
	contentType := previewContentType(head)
	if contentType == "" {
		return nil, "", ErrPreviewType
	}
	rest, err := io.ReadAll(io.LimitReader(reader, MaxPreviewFileBytes+1-int64(read)))
	if err != nil {
		return nil, "", err
	}
	contents := append(head, rest...)
	if len(contents) > MaxPreviewFileBytes {
		return nil, "", ErrPreviewTooLarge
	}
	return contents, contentType, nil
}

func (s Service) SaveText(
	ctx context.Context, alias, remotePath, contents, expectedRevision string,
) (TextFile, error) {
	if expectedRevision == "" {
		return TextFile{}, ErrRevisionRequired
	}
	if len(contents) > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	if !validText([]byte(contents)) {
		return TextFile{}, ErrNotUTF8
	}
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return TextFile{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return TextFile{}, err
	}
	defer remote.Close()
	// Saving through a symlink rewrites the file it points to and leaves the
	// link in place, as editors do.
	file, err := locateFile(remote, cleaned)
	if err != nil {
		return TextFile{}, err
	}

	current, err := readText(ctx, remote, file)
	if err != nil {
		return TextFile{}, err
	}
	if current.Revision != expectedRevision {
		return TextFile{}, ErrConflict
	}
	verify := func() error {
		latest, err := readText(ctx, remote, file)
		if errors.Is(err, fs.ErrNotExist) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		if latest.Revision != expectedRevision {
			return ErrConflict
		}
		return nil
	}
	if _, err := s.replace(
		ctx, remote, file.stored, strings.NewReader(contents), current.Entry.Mode.Perm(), 0, verify,
	); err != nil {
		return TextFile{}, err
	}
	return readText(ctx, remote, file)
}

func readText(ctx context.Context, remote Remote, file fileLocation) (TextFile, error) {
	before, err := remote.Lstat(file.stored)
	if err != nil {
		return TextFile{}, err
	}
	if !before.Mode().IsRegular() {
		return TextFile{}, ErrNotRegularFile
	}
	if before.Size() > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	opened, err := remote.Open(file.stored)
	if err != nil {
		return TextFile{}, err
	}
	contents, readErr := io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, reader: opened}, MaxEditableFileBytes+1))
	closeErr := opened.Close()
	if readErr != nil {
		return TextFile{}, readErr
	}
	if closeErr != nil {
		return TextFile{}, closeErr
	}
	if len(contents) > MaxEditableFileBytes {
		return TextFile{}, ErrTextTooLarge
	}
	if !validText(contents) {
		return TextFile{}, ErrNotUTF8
	}
	after, err := remote.Lstat(file.stored)
	if err != nil {
		return TextFile{}, err
	}
	if metadataRevision(before) != metadataRevision(after) {
		return TextFile{}, ErrConflict
	}
	entry := entryFrom(path.Dir(file.shown), namedInfo{FileInfo: after, name: path.Base(file.shown)})
	return TextFile{Entry: entry, Contents: string(contents), Revision: contentRevision(contents)}, nil
}

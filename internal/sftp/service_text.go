package sftp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"
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

// previewContentType は、先頭のバイト列だけからその型を返す。preview で
// 描いてよい型でなければ空を返す。名前が名乗る型は使わない。拡張子は
// 中身について何も保証しないからで、background 画像と同じ扱いである。
//
// 画像だけである。PDF は <iframe> でしか描けず、それには CSP へ blob: を
// 足す必要がある。画像は data: URL で足りるので、policy はそのままでよい。
//
// SVG もここに無い。<img> の中では script が動かないとはいえ、中身から
// 型が決まらない唯一の候補であり、background service も同じ理由で断る。
func previewContentType(head []byte) string {
	switch {
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return "image/jpeg"
	case len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return "image/webp"
	case len(head) >= 6 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return "image/gif"
	case len(head) >= 2 && head[0] == 'B' && head[1] == 'M':
		return "image/bmp"
	}
	return ""
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
	return Preview{Entry: entry, ContentType: contentType, Contents: contents, Revision: contentRevision(after, contents)}, nil
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
	return TextFile{Entry: entry, Contents: string(contents), Revision: contentRevision(after, contents)}, nil
}

func contentRevision(info fs.FileInfo, contents []byte) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, metadataRevision(info))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(contents)
	return "content-sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func validText(contents []byte) bool {
	return utf8.Valid(contents) && !bytes.ContainsRune(contents, '\x00')
}

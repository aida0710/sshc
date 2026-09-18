package sftp

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io/fs"
)

// Revision は API と UI が「この entry はまだ同じか」を確かめるための不透明な文字列。
// 種類は接頭辞で区別し、同じ接頭辞の値は同じ計算で作る。text・download・upload の
// content revision が同じ内容に対して一致することを、editor と転送の間で前提にする。
const (
	metadataRevisionPrefix = "meta-sha256:"
	contentRevisionPrefix  = "content-sha256:"
)

// metadataRevision は size・mode・mtime だけから作る。内容を読まずに「変わったか」を
// 判定する用途（一覧、rename、chmod、copy 後の照合）で使う。
func metadataRevision(info fs.FileInfo) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d\x00%d\x00%d\x00", info.Size(), info.Mode(), info.ModTime().UTC().UnixNano())
	return metadataRevisionPrefix + hex.EncodeToString(hash.Sum(nil))
}

// contentRevision は内容だけから作る。mtime が変わっても内容が同じなら一致する。
func contentRevision(contents []byte) string {
	sum := sha256.Sum256(contents)
	return contentRevisionPrefix + hex.EncodeToString(sum[:])
}

// contentRevisionOf は、stream しながら計算した SHA-256 から content revision を作る。
func contentRevisionOf(digest hash.Hash) string {
	return contentRevisionPrefix + hex.EncodeToString(digest.Sum(nil))
}

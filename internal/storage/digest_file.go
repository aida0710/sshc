package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// fileDigester は任意実装とする。実装しない FileSystem（テストの fake や、それを包む
// 障害注入）は、ReadFileLimited で読んでから Digest を求める従来の経路を通る。
type fileDigester interface {
	DigestFileLimited(path string, maximum int64) (string, error)
}

// privateFileDigester は、非公開状態を最終ハンドルで検証して読む privateFileReader の
// 実装のうち、中身を持たずにハッシュを求められるものが提供する。
type privateFileDigester interface {
	DigestPrivateFileLimited(path string, maximum int64) (string, error)
}

// DigestFileLimited は、シンボリックリンクをたどらずに通常ファイルを読み、Digest と
// 同じハッシュを、内容全体をメモリへ持たずに求める。maximum を超えるファイルは
// ErrFileTooLarge で断る。中身の要らない判定（同期の変更の有無など）が、大きな
// ファイルを丸ごと割り当てないために使う。
func DigestFileLimited(fileSystem FileSystem, path string, maximum int64) (string, error) {
	if maximum < 0 {
		return "", ErrFileTooLarge
	}
	if digester, ok := fileSystem.(fileDigester); ok {
		return digester.DigestFileLimited(path, maximum)
	}
	contents, err := ReadFileLimited(fileSystem, path, maximum)
	if err != nil {
		return "", err
	}
	return Digest(contents), nil
}

func (OSFileSystem) DigestFileLimited(path string, maximum int64) (string, error) {
	if maximum < 0 {
		return "", ErrFileTooLarge
	}
	file, err := openRegularNoFollow(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return digestBoundedRegularFile(file, maximum)
}

func (fileSystem workspaceFileSystem) DigestFileLimited(path string, maximum int64) (string, error) {
	if maximum < 0 {
		return "", ErrFileTooLarge
	}
	if !privateStateContains(fileSystem.stateDirectory, path) {
		return DigestFileLimited(fileSystem.FileSystem, path, maximum)
	}
	if digester, ok := fileSystem.privateReader.(privateFileDigester); ok {
		return digester.DigestPrivateFileLimited(path, maximum)
	}
	contents, err := fileSystem.ReadFileLimited(path, maximum)
	if err != nil {
		return "", err
	}
	return Digest(contents), nil
}

// digestBoundedRegularFile は、readBoundedRegularFile と同じ検査をして、中身を
// ハッシュへ流す。
func digestBoundedRegularFile(file *os.File, maximum int64) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotRegularFile
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maximum+1))
	if err != nil {
		return "", err
	}
	if written > maximum {
		return "", ErrFileTooLarge
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

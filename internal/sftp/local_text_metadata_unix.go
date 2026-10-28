//go:build linux || darwin

package sftp

import (
	"io/fs"
	"os"
	"syscall"
)

// unixLocalTextMetadata は、Linux と macOS で同じに読む部分である。stat の所有者と
// 権限、拡張属性を読み、通常ファイルでなければ断る。OS ごとの部分は呼び出し側が足す。
func unixLocalTextMetadata(file *os.File) (localTextMetadata, *syscall.Stat_t, error) {
	info, err := file.Stat()
	if err != nil {
		return localTextMetadata{}, nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return localTextMetadata{}, nil, ErrUnsupportedEntry
	}
	attributes, err := readLocalTextExtendedAttributes(file)
	if err != nil {
		return localTextMetadata{}, nil, err
	}
	return localTextMetadata{uid: int(stat.Uid), gid: int(stat.Gid), mode: info.Mode(), extendedAttributes: attributes}, stat, nil
}

// applyUnixLocalTextOwnership は、書きかけのファイルへ元の所有者・権限・拡張属性を当てる。
// 特権の付くビット（setuid・setgid・sticky）を持つファイルは、テキストの編集では置き換えない。
func applyUnixLocalTextOwnership(staged *os.File, snapshot localTextMetadata) error {
	if snapshot.mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky) != 0 {
		return ErrUnsupportedEntry
	}
	current, err := captureLocalTextMetadata(staged)
	if err != nil {
		return err
	}
	if current.uid != snapshot.uid || current.gid != snapshot.gid {
		if err := staged.Chown(snapshot.uid, snapshot.gid); err != nil {
			return err
		}
	}
	if err := staged.Chmod(snapshot.mode.Perm()); err != nil {
		return err
	}
	return applyLocalTextExtendedAttributes(staged, snapshot.extendedAttributes)
}

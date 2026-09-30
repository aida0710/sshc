package knownhosts

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"sshc/internal/storage"
)

// ReadFile は、ホスト鍵の照合に使う known_hosts のファイルひとつを読む。
//
// 無いファイルは空である。ワークスペース（~/.ssh）の中は、ほかの設定と同じく
// シンボリックリンクをたどらずに読む。それで読めないとき（シンボリックリンク、
// 通常のファイルでないもの、権限の無いもの）と、外のファイル（GlobalKnownHostsFile
// や、/dev/null を指す UserKnownHostsFile）は、OpenSSH と同じく読む
// （readFollowingLinks）。known_hosts はここでは読むだけで、書き込みはシンボリック
// リンクをたどらない storage が断る。Known Hosts 画面と鍵の追記は readInWorkspace で読む。
//
// path はホームの表記（~ や %d の展開結果）のままでよい。~/.ssh がシンボリック
// リンクのときも内側と判定できるよう、解決済みのルートの表記へ直してから比べる。
func ReadFile(workspace *storage.Workspace, path string) ([]byte, error) {
	path = workspace.Normalise(path)
	if !workspace.Contains(path) {
		return readFollowingLinks(path)
	}
	contents, err := workspace.FileSystem().ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case errors.Is(err, storage.ErrSymlinkPath), errors.Is(err, storage.ErrNotRegularFile), errors.Is(err, fs.ErrPermission):
		return readFollowingLinks(path)
	}
	return contents, err
}

// readInWorkspace は、sshc が書き換える known_hosts（Known Hosts 画面が見せて消すもの、
// 受け入れた鍵を足すもの）を読む。無いファイルは空である。
//
// 書き込みと同じく、シンボリックリンクをたどらずに読み、読めないことは誤りとして返す。
// 照合の ReadFile のように空として扱うと、画面は保存済みの鍵が無いように見せ、
// 利用者は鍵が消えたと誤解する。
func readInWorkspace(workspace *storage.Workspace, path string) ([]byte, error) {
	contents, err := workspace.FileSystem().ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return contents, err
}

// ErrFileTooLarge は、照合に使うには大きすぎる known_hosts を報告する。
//
// 空として扱って続けることはしない。そのファイルにある鍵と照合できないまま、
// accept-new で未知のホストとして鍵を受け入れることになるからである。
var ErrFileTooLarge = errors.New("known_hosts file is too large")

// maxFollowedFileSize は、シンボリックリンクをたどって読む known_hosts の量の上限である。
//
// 組織が GlobalKnownHostsFile で配るファイルは数万台分で数十 MiB になり、設定ファイル
// の上限（storage.MaxFileSize）では足りない。上限は、誤って巨大なファイルを指した
// ときにメモリを使い切らないためにある。
const maxFollowedFileSize = 64 << 20

// readFollowingLinks は、OpenSSH の load_hostkeys と同じく known_hosts を読む。
// シンボリックリンクはたどり、開けないファイルと通常のファイルでないものは空とする。
func readFollowingLinks(path string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !fileInfo.Mode().IsRegular() {
		return nil, nil
	}
	contents, err := io.ReadAll(io.LimitReader(file, maxFollowedFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxFollowedFileSize {
		return nil, ErrFileTooLarge
	}
	return contents, nil
}

package vpn

import (
	"errors"
	"os"
	"path/filepath"

	"sshc/internal/storage"
)

// 経路の置き場所（routeDirectory）のファイルを扱う。
//
// この場所はコンテナへ bind mount するので、コンテナの中からも書ける。コンテナの中の
// VPN クライアントが悪性の VPN サーバーに乗っ取られると、symlink、巨大なファイル、
// FIFO を置かれうる。engine はここのファイルを symlink をたどらず、上限まで、通常の
// ファイルだけ読む。

// prepareRouteDirectory は、経路の置き場所を利用者だけのものにする。
//
// 中継はTCPポートを開かない。ポートを開けば、このマシンの他の利用者が誰でも
// そのVPN経路へ乗れてしまう。engine の中継のソケットは0700のディレクトリの下に
// だけ置く。
func prepareRouteDirectory(directory string) error {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return err
	}
	// 前回の様子が残っていると、止まった経路の様子や失敗の理由を今のものとして
	// 見せてしまう。
	return removeRouteFiles(directory)
}

// routeFiles は、経路ひとつがホスト側に置くファイルである。
var routeFiles = []string{statusFileName, failureFileName, engineRelaySocketName}

// removeRouteFiles は、経路ひとつがホスト側に置いたファイルを消す。
func removeRouteFiles(directory string) error {
	for _, file := range routeFiles {
		if err := removeIfPresent(filepath.Join(directory, file)); err != nil {
			return err
		}
	}
	return nil
}

func removeIfPresent(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// readRouteFile は、agent が経路の置き場所に書いたファイルを、limit バイトまで読む。
// symlink、通常でないファイル（FIFO やデバイス）、limit を超えるファイルは読まずに
// 失敗を返す。FIFO を開いても待たない。
func (manager *Manager) readRouteFile(profileName, file string, limit int64) ([]byte, error) {
	return storage.ReadFileLimited(storage.OSFileSystem{}, filepath.Join(manager.routeDirectory(profileName), file), limit)
}

package sftp

import (
	"os"
	"time"
)

// LocalDownload は、エンジンのディスクにあるファイルを、ブラウザへ渡すために
// 開いたものである。呼んだ側が File を閉じる。
type LocalDownload struct {
	File     *os.File
	Name     string
	ModTime  time.Time
	Revision string
}

// OpenLocalDownload は、ローカルのシェルが出したパスのファイルを、そのまま
// 読める形で開く。SSH の接続先と違って転送のジョブも一時ファイルも要らない。
// 読み込み・プレビューと同じく、リンクとプライバシー保護の対象は断る。
func OpenLocalDownload(value string) (download LocalDownload, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	target, err := openLocalMutationTarget(value)
	if err != nil {
		return LocalDownload{}, err
	}
	defer target.parent.Close()
	metadata, err := inspectLocalReadPath(target.parent, target.name)
	if err != nil {
		return LocalDownload{}, err
	}
	info := metadata[target.name]
	if !info.Mode().IsRegular() {
		return LocalDownload{}, ErrNotRegularFile
	}
	opened, err := openStableLocalFile(localContentRead{root: target.parent, relative: target.name, metadata: metadata})
	if err != nil {
		return LocalDownload{}, err
	}
	return LocalDownload{File: opened.reader.(*os.File), Name: target.name, ModTime: info.ModTime(), Revision: metadataRevision(info)}, nil
}

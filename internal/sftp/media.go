package sftp

import (
	"context"
	"io"
	"path"
	"time"
)

// MediaFile は、再生のために開いた音声・動画である。Content は求められた
// 位置から読める。呼んだ側が Close する。
type MediaFile struct {
	Content     io.ReadSeeker
	ContentType string
	ModTime     time.Time
	close       func() error
}

func (file MediaFile) Close() error {
	return file.close()
}

// OpenMedia は、接続先の音声・動画を、途中から読める形で開く。ダウンロードと
// 違って一時ファイルへ写さない。再生は飛んだ先だけを読めば足りるからである。
func (s Service) OpenMedia(ctx context.Context, alias, remotePath string) (_ MediaFile, resultErr error) {
	contentType := MediaContentType(remotePath)
	if contentType == "" {
		return MediaFile{}, ErrPreviewType
	}
	cleaned, err := cleanPublicPath(remotePath, false)
	if err != nil {
		return MediaFile{}, err
	}
	remote, err := s.openRequest(ctx, alias)
	if err != nil {
		return MediaFile{}, err
	}
	defer func() {
		if resultErr != nil {
			closeTransferRemote(remote, resultErr)
		}
	}()
	ranged, ok := remote.(RangeRemote)
	if !ok {
		return MediaFile{}, ErrUnsupportedOperation
	}
	// A symlink plays the file it points to, as it downloads it.
	file, err := locateFile(remote, cleaned)
	if err != nil {
		return MediaFile{}, err
	}
	info, err := remote.Lstat(file.stored)
	if err != nil {
		return MediaFile{}, err
	}
	if !info.Mode().IsRegular() {
		return MediaFile{}, ErrNotRegularFile
	}
	content := &rangeReadSeeker{remote: ranged, path: file.stored, size: info.Size()}
	return MediaFile{
		Content:     content,
		ContentType: contentType,
		ModTime:     info.ModTime(),
		close: func() error {
			content.closeReader()
			return remote.Close()
		},
	}, nil
}

// OpenLocalMedia は、エンジンのディスクにある音声・動画を開く。読み込みと
// 同じく、リンクとプライバシー保護の対象は断る。
func OpenLocalMedia(localPath string) (MediaFile, error) {
	contentType := MediaContentType(path.Base(localPath))
	if contentType == "" {
		return MediaFile{}, ErrPreviewType
	}
	download, err := OpenLocalDownload(localPath)
	if err != nil {
		return MediaFile{}, err
	}
	return MediaFile{Content: download.File, ContentType: contentType, ModTime: download.ModTime, close: download.File.Close}, nil
}

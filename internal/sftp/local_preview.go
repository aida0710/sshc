package sftp

import (
	"context"
	"errors"
)

func ReadLocalPreview(ctx context.Context, value string) (preview Preview, err error) {
	defer func() { err = labelLocalAccessRefusal(err) }()
	target, err := openLocalMutationTarget(value)
	if err != nil {
		return Preview{}, err
	}
	defer target.parent.Close()
	read, err := readLocalBytes(ctx, localByteReadRequest{target: target, limit: MaxPreviewFileBytes})
	if errors.Is(err, ErrTextTooLarge) {
		return Preview{}, ErrPreviewTooLarge
	}
	if err != nil {
		return Preview{}, err
	}
	contentType := previewContentType(read.contents)
	if contentType == "" {
		return Preview{}, ErrPreviewType
	}
	return Preview{Entry: localMutationEntry(target.publicPath, read.metadata), ContentType: contentType, Contents: read.contents, Revision: contentRevision(read.contents)}, nil
}

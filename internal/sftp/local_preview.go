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
	contents, metadata, err := readLocalBytes(ctx, target, MaxPreviewFileBytes)
	if errors.Is(err, ErrTextTooLarge) {
		return Preview{}, ErrPreviewTooLarge
	}
	if err != nil {
		return Preview{}, err
	}
	contentType := previewContentType(contents)
	if contentType == "" {
		return Preview{}, ErrPreviewType
	}
	return Preview{Entry: localMutationEntry(target.publicPath, metadata), ContentType: contentType, Contents: contents, Revision: contentRevision(contents)}, nil
}

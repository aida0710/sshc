package sftp

import (
	"context"
	"fmt"
	"io/fs"
	"strings"
)

// isUploadPartName は、sshc が upload 中に置く sibling part file の名前かを返す。
func isUploadPartName(name string) bool {
	return uploadPartNamePattern.MatchString(name)
}

// isInternalName は、sshc 自身が remote に置く一時 file（upload part と editor の
// 一時 file）の名前かを返す。一覧からは隠し、削除や移動では衝突として扱う。
func isInternalName(name string) bool {
	return isUploadPartName(name) || editorTemporaryNamePattern.MatchString(name)
}

// validRemoteChildName は、server が返した entry 名を親 directory の配下に留められる
// かを返す。pkg/sftp は server が返す "x/.." を path.Base で ".." に、空文字を "." に
// 縮めるため、悪性または壊れた server はこの検査なしに別の path へ誘導できる。
func validRemoteChildName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00")
}

// validLocalChildName は、remote の entry 名を engine の local file system へそのまま
// 書けるかを返す。Windows の区切り文字は OS に関係なく拒否し、同じ tree が OS ごとに
// 違う場所へ展開されないようにする。
func validLocalChildName(name string) bool {
	return validRemoteChildName(name) && !strings.ContainsRune(name, '\\')
}

// readChildren は directory の entry を読み、server が返した名前を信用せずに検査する。
// 不正な名前は listing 全体の失敗にする。黙って飛ばすと、利用者は存在する entry を
// 見落とし、copy や delete は別の tree を処理したことに気付けない。
func readChildren(ctx context.Context, remote Remote, directory string) ([]fs.FileInfo, error) {
	infos, err := remote.ReadDir(ctx, directory)
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		if !validRemoteChildName(info.Name()) {
			return nil, fmt.Errorf("%w: server returned entry name %q in %s", ErrInvalidPath, info.Name(), directory)
		}
	}
	return infos, nil
}

package sftp

import (
	"errors"
	"io/fs"
	"path"
	"strings"
)

// refuseTargetInsideSource は、フォルダの copy・move の転送先が、転送元のフォルダ自身か
// その配下にあれば ErrTargetInsideSource を返す。転送先には何も書かない。
//
// 配下への copy は、自分で作ったフォルダを転送元としてまた読み、木の上限に当たるまで
// 入れ子を作り続ける。転送元と同じフォルダへの move は、別の alias からだと各ファイルを
// 自分の上へ copy してから転送元を消すので、唯一の中身を消す。
//
// パスの文字列だけでは、転送先の途中のシンボリックリンクが転送元の配下を指していても
// 分からない。そこで転送元と、転送先の親を、サーバーの REALPATH で解決してから比べる
// （resolveTransferPaths）。転送先そのものは解決しない。まだ無いことが多く、あっても
// シンボリックリンクなら、copy・move は種類の違う項目として上書きせずに断る。
func (s Service) refuseTargetInsideSource(request RemoteTransferRequest, source, target Remote, sourcePath, targetPath string) error {
	sourceLocation, targetLocation := sourcePath, targetPath
	if resolvedSource, resolvedTarget, resolved := resolveTransferPaths(source, target, sourcePath, targetPath); resolved {
		sourceLocation, targetLocation = resolvedSource, resolvedTarget
	}
	if !isSameOrDescendant(sourceLocation, targetLocation) {
		return nil
	}
	if request.SourceAlias == request.TargetAlias {
		return ErrTargetInsideSource
	}
	// 別の alias（web と web-admin など）が同じサーバーを指すことがあり、alias の一致では
	// 分からない。パスが重なって見えても、別のサーバーなら転送してよい。
	sameServer, err := s.targetVisibleFromSource(source, target, targetLocation)
	if err != nil {
		return err
	}
	if sameServer {
		return ErrTargetInsideSource
	}
	return nil
}

// resolveTransferPaths は、転送元のパスと転送先のパスを、サーバーが解決した絶対パスに
// して返す。転送先は、親を解決して名前を付け直す。どちらかを解決できなければ resolved は
// false で、呼び出し側は要求のパスの文字列で比べる。片方だけ解決したパスと比べると、
// 同じ場所を別の名前で比べて見逃すからである。
//
// 解決できないことを理由に断らない。REALPATH は SFTP v3 の必須の要求で、失敗するのは
// 途中のフォルダが無いか、たどる権限が無いときである。そのときは copy・move も同じ場所で
// 失敗し、利用者には本当の理由（見つからない、権限が無い）が出る。断ると、自分の配下への
// 転送という誤った理由を見せることになる。文字列の比較で見逃したシンボリックリンクがあっても、
// copy は木の上限（maxTransferTreeEntries）で止まる。
func resolveTransferPaths(source, target Remote, sourcePath, targetPath string) (string, string, bool) {
	resolvedSource, err := resolveRealPath(source, sourcePath)
	if err != nil {
		return "", "", false
	}
	resolvedParent, err := resolveRealPath(target, path.Dir(targetPath))
	if err != nil {
		return "", "", false
	}
	return resolvedSource, path.Join(resolvedParent, path.Base(targetPath)), true
}

// resolveRealPath は、サーバーの REALPATH の値を、比べられる絶対パスにして返す。サーバーの
// 値は信用せず、絶対パスでない値や NUL を含む値は失敗として扱う。
func resolveRealPath(remote Remote, remotePath string) (string, error) {
	resolved, err := remote.RealPath(remotePath)
	if err != nil {
		return "", err
	}
	return cleanPath(resolved, true)
}

// isSameOrDescendant は、candidate が folder と同じパスか、その配下のパスかを返す。
// 名前の前方が一致するだけの別のフォルダ（"/a/b" に対する "/a/bc"）は含まない。
func isSameOrDescendant(folder, candidate string) bool {
	return candidate == folder || strings.HasPrefix(candidate, strings.TrimSuffix(folder, "/")+"/")
}

// targetVisibleFromSource は、target の接続で targetPath の場所に作ったファイルが、source の
// 接続から同じパスに見えるかを返す。見えれば、2つの接続は同じサーバーの同じ場所を指す。
//
// SFTP は inode を返さないので、target から一時ファイルの名前で空のファイルを作って確かめる。
// 消し損ねても、一時ファイルの名前なので一覧に出ず、abandonedTemporaryAge を過ぎれば
// 置き去りとして扱われる。
func (s Service) targetVisibleFromSource(source, target Remote, targetPath string) (bool, error) {
	// copy が書き込む場所に作る。統合先のフォルダがあればその中、無ければ copy が
	// フォルダを作る親の中である。
	directory := path.Dir(targetPath)
	if existing, err := target.Lstat(targetPath); err == nil && existing.IsDir() {
		directory = targetPath
	}
	probe, err := s.temporaryPath(path.Join(directory, "sshc-probe"))
	if err != nil {
		return false, err
	}
	written, err := target.Create(probe)
	if err != nil {
		return false, err
	}
	if err := written.Close(); err != nil {
		return false, errors.Join(err, target.Remove(probe))
	}
	_, seenErr := source.Lstat(probe)
	if err := target.Remove(probe); err != nil {
		return false, err
	}
	switch {
	case seenErr == nil:
		return true, nil
	case errors.Is(seenErr, fs.ErrNotExist):
		return false, nil
	default:
		return false, seenErr
	}
}

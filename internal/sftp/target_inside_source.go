package sftp

import (
	"errors"
	"io/fs"
	"path"
	"strings"
)

// remoteTransferEnds は、remote 間の copy・move の両端である。source と target は開いた
// 接続で、同じ alias なら同じ接続を指す。パスは cleanRemoteTransferRequest で整えたもの。
type remoteTransferEnds struct {
	source, target         Remote
	sourcePath, targetPath string
	sameAlias              bool
	// overwrite は、利用者が転送先の上書きを承認したか。
	overwrite bool
}

// refuseTransferOntoSource は、転送先が転送元と重なる転送を、転送先を変える前に断る。
// フォルダは refuseTargetInsideSource で判定する。ファイルは、同じ alias の同じパスへの
// 転送を既存の項目（ErrAlreadyExists）として扱う。
func (s Service) refuseTransferOntoSource(ends remoteTransferEnds, sourceInfo fs.FileInfo) error {
	if sourceInfo.IsDir() {
		return s.refuseTargetInsideSource(ends)
	}
	if ends.sameAlias && ends.sourcePath == ends.targetPath {
		return ErrAlreadyExists
	}
	return nil
}

// refuseTargetInsideSource は、フォルダの copy・move の転送先が、転送元のフォルダ自身か
// その配下にあれば ErrTargetInsideSource を返す。断ったときは、転送先に何も残さない。
// 別の alias では、同じサーバーかを確かめるために転送先へ一時ファイルを作り、すぐ消す
// （targetVisibleFromSource）。
//
// 配下への copy は、自分で作ったフォルダを転送元としてまた読み、木の上限に当たるまで
// 入れ子を作り続ける。転送元と同じフォルダへの move は、別の alias からだと各ファイルを
// 自分の上へ copy してから転送元を消すので、唯一の中身を消す。同じ alias の同じパスも、
// 上書きの確認に回さずに断る。上書きを承認しても転送できないからである。
func (s Service) refuseTargetInsideSource(ends remoteTransferEnds) error {
	comparedSourcePath, comparedTargetPath := comparablePaths(ends)
	if !isSameOrDescendant(comparedSourcePath, comparedTargetPath) {
		return nil
	}
	if ends.sameAlias {
		return ErrTargetInsideSource
	}
	// 別の alias（web と web-admin など）が同じサーバーを指すことがあり、alias の一致では
	// 分からない。パスが重なって見えても、別のサーバーなら転送してよい。確かめるには
	// 転送先へ書くので、上書きの承認が要る転送は、承認の前に既存の転送先へ書かないよう
	// ここで確認に回し、承認したあとの実行で確かめる。
	existing, err := ends.target.Lstat(comparedTargetPath)
	targetExists := err == nil
	if targetExists && !ends.overwrite {
		return ErrAlreadyExists
	}
	// copy が書き込む場所に作る。統合先のフォルダがあればその中、無ければ copy が
	// フォルダを作る親の中である。
	probeDirectory := path.Dir(comparedTargetPath)
	if targetExists && existing.IsDir() {
		probeDirectory = comparedTargetPath
	}
	sameServer, err := s.targetVisibleFromSource(ends, probeDirectory)
	if err != nil {
		return err
	}
	if sameServer {
		return ErrTargetInsideSource
	}
	return nil
}

// comparablePaths は、重なりを比べる転送元と転送先のパスを返す。パスの文字列だけでは、
// 途中のシンボリックリンクが転送元の配下を指していても分からない。そこで両方を
// サーバーの REALPATH で解決する。転送先は、親を解決して名前を付け直す。転送先そのものは
// 解決しない。まだ無いことが多く、あってもシンボリックリンクなら、copy・move は種類の
// 違う項目として上書きせずに断る。
//
// どちらかを解決できなければ、両方とも要求のパスを返す。片方だけ解決したパスと比べると、
// 同じ場所を別の名前で比べて見逃すからである。
//
// 解決できないことを理由には断らない。REALPATH は SFTP v3 の必須の要求で、失敗するのは
// 途中のフォルダが無いか、たどる権限が無いときである。そのときは copy・move も同じ場所で
// 失敗し、汎用の失敗（sftp_failed）になる。断ると、自分の配下への転送という誤った理由を
// 見せる。文字列の比較で見逃したシンボリックリンクがあっても、copy は木の上限
// （maxTransferTreeEntries）で止まる。
func comparablePaths(ends remoteTransferEnds) (comparedSourcePath, comparedTargetPath string) {
	resolvedSourcePath, err := resolveRealPath(ends.source, ends.sourcePath)
	if err != nil {
		return ends.sourcePath, ends.targetPath
	}
	resolvedTargetParent, err := resolveRealPath(ends.target, path.Dir(ends.targetPath))
	if err != nil {
		return ends.sourcePath, ends.targetPath
	}
	return resolvedSourcePath, path.Join(resolvedTargetParent, path.Base(ends.targetPath))
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

// targetVisibleFromSource は、target の接続で probeDirectory に作ったファイルが、source の
// 接続から同じパスに見えるかを返す。見えれば、2つの接続は同じサーバーの同じ場所を指す。
//
// SFTP は inode を返さないので、target から一時ファイルの名前で空のファイルを作って確かめ、
// すぐ消す。消し損ねても、一時ファイルの名前なので一覧に出ず、abandonedTemporaryAge を
// 過ぎれば置き去りとして扱われる。
func (s Service) targetVisibleFromSource(ends remoteTransferEnds, probeDirectory string) (bool, error) {
	probe, err := s.temporaryPath(path.Join(probeDirectory, "sshc-probe"))
	if err != nil {
		return false, err
	}
	written, err := ends.target.Create(probe)
	if err != nil {
		return false, err
	}
	if err := written.Close(); err != nil {
		return false, errors.Join(err, ends.target.Remove(probe))
	}
	_, seenErr := ends.source.Lstat(probe)
	if err := ends.target.Remove(probe); err != nil {
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

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
// フォルダは自身か配下への転送（refuseTargetInsideSource）を、ファイルは自身への転送
// （refuseTargetIsSource）を断る。ほかの種類の項目（シンボリックリンクなど）は、計画
// （PlanRemoteTransfer）が ErrUnsupportedEntry で断るので比べない。転送元がリンクだと、
// REALPATH はリンクの先を返すので、リンクそのものとは比べられない。
func (s Service) refuseTransferOntoSource(ends remoteTransferEnds, sourceInfo fs.FileInfo) error {
	switch {
	case sourceInfo.IsDir():
		return s.refuseTargetInsideSource(ends)
	case sourceInfo.Mode().IsRegular():
		return s.refuseTargetIsSource(ends, sourceInfo)
	default:
		return nil
	}
}

// refuseTargetInsideSource は、フォルダの copy・move の転送先が、転送元のフォルダ自身か
// その配下にあれば ErrTargetInsideSource を返す。断ったときは、転送先に何も残さない。
// 別の alias では、同じサーバーかを確かめるために転送先へ一時ファイルを作り、すぐ消す
// （refuseOnTheSameServer）。
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
	existing, err := lstatIfExists(ends.target, comparedTargetPath)
	if err != nil {
		return err
	}
	// copy が書き込む場所に作る。統合先のフォルダがあればその中、無ければ copy が
	// フォルダを作る親の中である。
	probeDirectory := path.Dir(comparedTargetPath)
	if existing != nil && existing.IsDir() {
		probeDirectory = comparedTargetPath
	}
	return s.refuseOnTheSameServer(ends, sameServerCheck{
		targetExists: existing != nil, probeDirectory: probeDirectory, refusal: ErrTargetInsideSource,
	})
}

// refuseTargetIsSource は、ファイルの copy・move の転送先が、転送元のファイルそのもの
// なら ErrTargetIsSource を返す。断ったときは、転送先に何も残さない。
//
// 別の alias から同じファイルへの move は、自分の上へ copy してから転送元を消すので、
// 唯一のファイルを消す。同じ alias の move は、同じファイルへの rename が何もせずに成功し、
// 移したように見えるだけである。copy も同じ内容で置き換えるだけで、何も転送しない。
// 同じ alias の同じパスは、上書きの確認に回さずに断る。上書きを承認しても転送できない
// からである。
//
// 別の alias では、転送先が転送元と別のファイルだと分かれば、一時ファイルを作らずに
// 通す。転送先に何も無いときと、サイズか更新日時が sourceInfo と違うときである。
func (s Service) refuseTargetIsSource(ends remoteTransferEnds, sourceInfo fs.FileInfo) error {
	comparedSourcePath, comparedTargetPath := comparablePaths(ends)
	if comparedTargetPath != comparedSourcePath {
		return nil
	}
	if ends.sameAlias {
		return ErrTargetIsSource
	}
	existing, err := lstatIfExists(ends.target, comparedTargetPath)
	if err != nil {
		return err
	}
	// 転送先に何も無ければ、上書きされる転送元も無い。
	if existing == nil {
		return nil
	}
	// 同じファイルなら、どちらの接続からも同じ stat のサイズと更新日時（SFTP では秒）が
	// 見えるので、違えば別のファイルである。比較の画面の同期は、別のサーバーの同じパスに
	// ある違うファイルへの上書きをファイルごとに積むので、ここで一時ファイルの作成と削除を
	// 省く。比べるのは、同じファイルなら必ず同じになるサイズと更新日時だけにする。違って
	// 見えるだけで通すと、同じファイルを見逃して転送元を消すからである。sourceInfo を
	// 読んだあとで転送元が変わって違って見えた場合は、copyFile が前後の照合で失敗にし、
	// 公開しない。
	if existing.Size() != sourceInfo.Size() || !existing.ModTime().Equal(sourceInfo.ModTime()) {
		return nil
	}
	return s.refuseOnTheSameServer(ends, sameServerCheck{
		targetExists: true, probeDirectory: path.Dir(comparedTargetPath), refusal: ErrTargetIsSource,
	})
}

// sameServerCheck は、別の alias の転送先が転送元と重なって見えるときに、同じサーバーか
// を確かめる材料である。
type sameServerCheck struct {
	// targetExists は、転送先に既存の項目があるか。あれば、上書きの承認が要る。
	targetExists bool
	// probeDirectory は、一時ファイルを作るフォルダ。copy が書き込む場所にする。
	probeDirectory string
	// refusal は、同じサーバーだったときに返す失敗。
	refusal error
}

// refuseOnTheSameServer は、別の alias の転送先が転送元と同じサーバーにあれば
// check.refusal を返す。別の alias（web と web-admin など）が同じサーバーを指すことがあり、
// alias の一致では分からない。パスが重なって見えても、別のサーバーなら転送してよい。
//
// 確かめるには転送先へ一時ファイルを書く（targetVisibleFromSource）。そこで、既存の
// 転送先への上書きがまだ承認されていなければ、書かずに ErrAlreadyExists を返して上書きの
// 確認に回し、承認したあとの実行で確かめる。承認の前に、既存の項目の隣や中へ書かない
// ためである。
func (s Service) refuseOnTheSameServer(ends remoteTransferEnds, check sameServerCheck) error {
	if check.targetExists && !ends.overwrite {
		return ErrAlreadyExists
	}
	sameServer, err := s.targetVisibleFromSource(ends, check.probeDirectory)
	if err != nil {
		return err
	}
	if sameServer {
		return check.refusal
	}
	return nil
}

// lstatIfExists は、remotePath の項目を返す。無ければ nil を返す。無いこと以外の理由で
// 確かめられなければ、その失敗を返す。無いとみなして進むと、既存の項目への上書きが
// 承認される前に、同じサーバーかを確かめる一時ファイルを書きうる。ファイルでは、確かめ
// ずに通した同じファイルへの move が、直後の copy で転送先を見つけて転送元を消しうる。
func lstatIfExists(remote Remote, remotePath string) (fs.FileInfo, error) {
	info, err := remote.Lstat(remotePath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return info, err
}

// comparablePaths は、重なりを比べる転送元と転送先のパスを返す。パスの文字列だけでは、
// 途中のシンボリックリンクが転送元の配下を指していても分からない。そこで両方を
// サーバーの REALPATH で解決する。転送先は、親を解決して名前を付け直す。転送先そのものは
// 解決しない。まだ無いことが多く、あってもシンボリックリンクなら、copy・move はリンクの
// 先へ書かない。フォルダの転送は種類の違う項目として上書きせずに断り、ファイルの転送は
// 承認された上書きでリンクそのものを置き換える。
//
// どちらかを解決できなければ、両方とも要求のパスを返す。片方だけ解決したパスと比べると、
// 同じ場所を別の名前で比べて見逃すからである。
//
// 解決できないことを理由には断らない。REALPATH は SFTP v3 の必須の要求で、失敗するのは
// 途中のフォルダが無いか、たどる権限が無いときである。そのときは copy・move も同じ場所で
// 失敗し、汎用の失敗（sftp_failed）になる。断ると、自分自身か配下への転送という誤った
// 理由を見せる。文字列の比較で見逃したシンボリックリンクがあっても、フォルダの copy は
// 木の上限（maxTransferTreeEntries）で止まる。
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

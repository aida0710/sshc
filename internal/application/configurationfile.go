package application

import (
	"strings"

	"sshc/internal/keys"
)

// nonConfigurationFileNames は、OpenSSHが~/.sshの中で設定ファイル以外の意味を与えている
// ファイル名である。中身がまだ空でも、この名前のファイルを設定として書かない。
var nonConfigurationFileNames = []string{
	"authorized_keys",
	"authorized_keys2",
	"known_hosts",
	"known_hosts2",
}

// readConfigurationFile は、Config Explorerのファイル操作（読み込み、ファイル全体の
// 編集、改名の元、削除）が設定ファイルとして扱う absolute を読む。設定ファイルでなければ
// ErrNotEditable を返し、中身を返さない。まだ無いファイルは exists が false になる。
//
// 名前で断れるもの（sshcの状態ファイルや鍵のディレクトリ）は読む前に断る。状態ファイルを
// 読んでから断ると、断るだけの操作でVaultの鍵を読み、Windowsでは状態ファイルのACLの
// 確認に落ちたときにACLのエラーが返ってしまう。名前で決まらないものは、中身が鍵や
// known_hostsかで断る。
func (s *Service) readConfigurationFile(absolute string) (contents []byte, exists bool, err error) {
	if err := s.checkConfigurationPath(absolute); err != nil {
		return nil, false, err
	}
	contents, exists, err = s.readFile(absolute)
	if err != nil {
		return nil, false, err
	}
	switch keys.ClassifyContents(contents) {
	case keys.KindPrivateKey, keys.KindPublicKey, keys.KindCertificate, keys.KindKnownHosts:
		return nil, false, ErrNotEditable
	}
	return contents, exists, nil
}

// checkConfigurationPath は、absolute が名前だけで設定ファイルでないと決まるものか
// （sshcの状態ディレクトリと keys/ の下、known_hosts など）を確かめ、そうなら
// ErrNotEditable を返す。改名先のように、まだ中身の無いパスにも使う。
//
// この境界をUIの一覧だけに任せると、新規ファイル欄や改名欄に打ち込んだ名前で、
// sshcの状態ファイル（local-vault-keyなど）を書き換えたり、衝突の差分に秘密鍵の
// 本文を載せたりできてしまう。比較で大小文字を区別しないのは、既定のmacOSと
// Windowsのボリュームが "Keys" と "keys" を同じ場所として扱うためである。
func (s *Service) checkConfigurationPath(absolute string) error {
	relative, err := RelativePath(s.workspace.Root(), absolute)
	if err != nil {
		return err
	}
	segments := strings.Split(relative, "/")
	if s.isReservedDirectory(segments[0]) {
		return ErrNotEditable
	}
	name := segments[len(segments)-1]
	for _, reserved := range nonConfigurationFileNames {
		if strings.EqualFold(name, reserved) {
			return ErrNotEditable
		}
	}
	return nil
}

// isReservedDirectory は、~/.ssh直下の name が、設定ではないものを置くディレクトリ
// （sshcの状態ディレクトリと、グループの鍵を置く keys/）かを報告する。
func (s *Service) isReservedDirectory(name string) bool {
	stateDirectory, err := RelativePath(s.workspace.Root(), s.workspace.StateDir())
	if err == nil && strings.EqualFold(name, stateDirectory) {
		return true
	}
	return strings.EqualFold(name, KeysDirectory)
}

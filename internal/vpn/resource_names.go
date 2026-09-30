package vpn

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strconv"
)

// コンテナと、ホスト側の経路の置き場所の名前を決める。
//
// プロファイル名はそのまま使わない。名前には空白や日本語を使えるが、コンテナ名に
// 使える字は限られる。また macOS と Windows のファイルシステムは既定で大文字と
// 小文字を区別しないので、「Lab」と「lab」の経路の置き場所が重なる。名前から
// 決まる固定長の識別子を使えば、どちらも起きない。コンテナがどのプロファイルの
// ものかは、名前そのものを持つ札（profileLabel）で見分ける。

const (
	// workspaceIdentityLength は、workspace の識別子に使うハッシュの桁数である。
	// コンテナ名に入るので短くし、衝突しない程度には長くする。
	workspaceIdentityLength = 12
	// profileIdentifierLength は、プロファイルの識別子の桁数である。プロファイルは
	// 64 件まで（API の vpnProfiles）なので、64 bit あれば重ならない。ソケットの
	// パスの上限（maxSocketPathLength）に収まるよう、これ以上は長くしない。
	profileIdentifierLength = 16
)

// hexDigest は、text の SHA-256 の16進表記の先頭 length 桁を返す。
func hexDigest(text string, length int) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])[:length]
}

// workspaceIdentity は、中継を置く directory から workspace の識別子を作る。
// 同じ workspace で起動し直した engine は、同じ識別子になる。
func workspaceIdentity(directory string) string {
	return hexDigest(directory, workspaceIdentityLength)
}

// profileIdentifier は、プロファイル名から決まる識別子である。英小文字と数字だけで
// できている。大文字と小文字だけが違う名前も、別の識別子になる。
func profileIdentifier(profileName string) string {
	return hexDigest(profileName, profileIdentifierLength)
}

// containerName は、この利用者のこの workspace のこのプロファイルのコンテナ名である。
func (manager *Manager) containerName(profileName string) string {
	return "sshc-vpn-" + profileIdentifier(profileName) + "-" + strconv.Itoa(manager.owner) + "-" + manager.workspace
}

// routeDirectory は、このプロファイルの経路がホスト側に置く場所である。engine の
// 中継のソケットと、agent が書くトンネルの様子と失敗の理由がここに置かれる。
func (manager *Manager) routeDirectory(profileName string) string {
	return filepath.Join(manager.directory, profileIdentifier(profileName))
}

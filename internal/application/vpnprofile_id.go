package application

import (
	"crypto/rand"
	"encoding/hex"
)

// VPN プロファイルの識別子（VPNProfile.ID）を決めて確かめる。
//
// 保存済みのパスワード・TOTP・起動スニペットの割り当ては、接続に付けたプロファイルを
// 結び付けの値（sshclient.Target.AuthenticationBinding）に入れる。名前は表示名なので
// 改名で変わり、削除して同じ名前で作り直したプロファイルは別のサーバーの別の VPN で
// ありうる。そこで、作るときに乱数で決め、改名と編集では変えない識別子を入れる。
// 結び付けの値は改名では変わらず、作り直せば、どの時点で外したかにかかわらず必ず
// 変わる。

// vpnProfileIDBytes は、識別子のバイト数である。128 bit の乱数なら、作り直した
// プロファイルが前の識別子を引き当てることはない。
const vpnProfileIDBytes = 16

// newVPNProfileID は、作るプロファイルの識別子を乱数で決める。
func newVPNProfileID() (string, error) {
	raw := make([]byte, vpnProfileIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// validVPNProfileID は、識別子が vpnProfileIDBytes バイトの小文字の16進かを返す。
func validVPNProfileID(id string) bool {
	if len(id) != 2*vpnProfileIDBytes {
		return false
	}
	for _, character := range id {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

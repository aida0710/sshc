package application

import (
	"crypto/rand"
	"crypto/sha256"
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

// migratedVPNProfileIDDomain は、名前から識別子を決めるときに名前の前に置く。経路の
// コンテナ名も名前の SHA-256 から作る（internal/vpn の resource_names.go）ので、同じ
// 値にしない。
const migratedVPNProfileIDDomain = "sshc vpn profile id\x00"

// newVPNProfileID は、作るプロファイルの識別子を乱数で決める。
func newVPNProfileID() (string, error) {
	raw := make([]byte, vpnProfileIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

// migratedVPNProfileID は、schema 9 より前の metadata のプロファイルに与える識別子である。
//
// 移行は metadata を読むたびに走り、書き直すまで結果は残らない。乱数にすると、読むたびに
// 識別子が変わって割り当てが合わなくなるので、名前から決める。前の形では名前が1つの
// プロファイルを指していたので、名前から決めても重ならない。作り直したプロファイルは
// 乱数の識別子を持つので、移行した識別子とも重ならない。
func migratedVPNProfileID(name string) string {
	digest := sha256.Sum256([]byte(migratedVPNProfileIDDomain + name))
	return hex.EncodeToString(digest[:vpnProfileIDBytes])
}

// giveVPNProfilesMigratedIDs は、識別子を持たないプロファイルに、名前から決めた識別子を
// 与える。schema 9 より前の metadata の移行だけが呼ぶ。
func giveVPNProfilesMigratedIDs(metadata *Metadata) {
	for index := range metadata.VPNProfiles {
		if stored := &metadata.VPNProfiles[index]; stored.ID == "" {
			stored.ID = migratedVPNProfileID(stored.Name)
		}
	}
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

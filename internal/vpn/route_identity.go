package vpn

import "crypto/sha256"

// 動いているコンテナが、いまの設定とシークレットのままの経路かを見分ける。
//
// どちらかが変わったコンテナは作り直す。古いまま繋ぎ続けると、利用者が直した設定ファイル
// （OpenVPN と WireGuard では、設定ファイルはシークレットである）や鍵が、経路を止めるまで
// 使われない。

// routeIdentity は、コンテナが提供している経路を見分けるものである。シークレットは要約だけを
// 持ち、値そのものは engine のメモリに残さない。
type routeIdentity struct {
	profile Profile
	// secretsDigest は、方式が使うシークレットの記録（EncodeSecrets）の SHA-256 である。
	secretsDigest [sha256.Size]byte
}

// newRouteIdentity は、この設定とシークレットで作る経路を見分けるものを作る。
func newRouteIdentity(profile Profile, secrets Secrets) routeIdentity {
	// 文字列だけの構造体の JSON なので、EncodeSecrets は失敗しない。
	encoded, _ := EncodeSecrets(profile.OwnSecrets(secrets))
	return routeIdentity{profile: profile, secretsDigest: sha256.Sum256([]byte(encoded))}
}

// same は、ふたつが同じ経路を作るかを返す。
func (identity routeIdentity) same(other routeIdentity) bool {
	return identity.profile.sameRouteAs(other.profile) && identity.secretsDigest == other.secretsDigest
}

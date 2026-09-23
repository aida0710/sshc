package vpn

import (
	"fmt"
	"time"

	"sshc/internal/totp"
)

// openconnect backend の設定を組み立てる。
//
// openconnect は接続のたびに外部スクリプトを呼び、そこで interface と経路を
// 作らせる。既定の vpnc-script は装置が配った既定経路とDNSをそのまま入れる。
// それでは、このコンテナが「接続先ひとつだけを通す」約束を守れない。だから
// 自分の script（container/vpnc-script）をイメージに焼き、それを呼ばせる。

// defaultOpenConnectProtocol は、方式を書かなかったときに使う方式である。
// Cisco AnyConnect と、それに合わせた装置（ocserv など）が話す。
const defaultOpenConnectProtocol = "anyconnect"

// openConnectProtocol は、profile が指定した方式、または既定を返す。
func openConnectProtocol(settings OpenConnectSettings) string {
	if settings.Protocol == "" {
		return defaultOpenConnectProtocol
	}
	return settings.Protocol
}

// secondFactorAnswer は、装置が二段目に聞いてくることへの答えを作る。
//
// openconnect は、パスワードを標準入力から読んだあと、次の質問にも標準入力の
// 次の行を使う。フォームの名前も項目の名前も装置ごとに違うので、そこへは触れ
// ない。答えが空なら、二段目を聞かない装置だということである。
//
// TOTP のコードは 30 秒で変わる。コンテナへ渡す直前に作る。
func secondFactorAnswer(settings OpenConnectSettings, secrets Secrets, now time.Time) (string, error) {
	switch settings.SecondFactor {
	case "":
		return "", nil
	case SecondFactorApprove:
		// 何も聞かずに通知だけ出す装置には、送る語が無い。空の行を送ると、
		// 二段目を聞く装置に「空の答え」を渡すことになる。
		return settings.ApprovalWord, nil
	case SecondFactorTOTP:
		config, err := totp.Parse(secrets.OpenConnectTOTPSecret)
		if err != nil {
			return "", fmt.Errorf("%w: 二段目のTOTPの種が読めません", ErrSecrets)
		}
		code, err := config.Code(now)
		if err != nil {
			return "", fmt.Errorf("%w: 二段目のコードを作れません", ErrSecrets)
		}
		return code, nil
	}
	return "", fmt.Errorf("%w: 二段目の答え方 %q は知りません", ErrSettings, settings.SecondFactor)
}

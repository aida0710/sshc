package sshclient

import "errors"

// passwordOffer makes a saved account password single-use across password and
// keyboard-interactive while retaining whether a secret was actually offered.
// A second callback means the server rejected that offer.
type passwordOffer struct {
	target   Target
	provider func(target Target) (string, bool)
	checked  bool
	offered  bool
	// rejected は、送ったあとでサーバーがもう一度パスワードを尋ねたことである。
	rejected bool
}

// noteAsked は、サーバーがパスワードを尋ねたことを記録し、送ったパスワードが
// 拒否されていたかを返す。送る前に尋ねられただけなら拒否ではない。
func (offer *passwordOffer) noteAsked() bool {
	if offer.wasOffered() {
		offer.rejected = true
	}
	return offer.wasRejected()
}

func (offer *passwordOffer) wasRejected() bool { return offer != nil && offer.rejected }

func (offer *passwordOffer) take() (string, bool) {
	if offer == nil || offer.checked || offer.provider == nil || offer.target.Alias == "" {
		return "", false
	}
	offer.checked = true
	password, found := offer.provider(offer.target)
	offer.offered = found
	return password, found
}

func (offer *passwordOffer) wasOffered() bool { return offer != nil && offer.offered }

// totpOffer は、保存済みTOTPのコードを接続ごとに1回だけ出す。
//
// 2回目にTOTPを聞かれたなら、送ったコードは断られている。同じ時間窓では同じ
// コードができるので、送り直してもまた断られ、サーバーの試行回数の制限を
// 使い切るだけである。2回目からは利用者に尋ねる。
type totpOffer struct {
	target   Target
	provider func(target Target, question string) (string, bool)
	offered  bool
	// rejected は、送ったあとでサーバーがもう一度TOTPを尋ねたことである。
	rejected bool
}

// noteAsked は、サーバーがTOTPを尋ねたことを記録し、送ったコードが拒否されて
// いたかを返す。送る前に尋ねられただけなら拒否ではない。
func (offer *totpOffer) noteAsked() bool {
	if offer.offered {
		offer.rejected = true
	}
	return offer.wasRejected()
}

func (offer *totpOffer) wasRejected() bool { return offer != nil && offer.rejected }

func (offer *totpOffer) take(question string) (string, bool) {
	if offer.offered || offer.provider == nil {
		return "", false
	}
	code, found := offer.provider(offer.target, question)
	offer.offered = found
	return code, found
}

func (offer *totpOffer) wasOffered() bool { return offer.offered }

// storedCredentials は、ひとつの接続で自動入力に使う保存済みの資格情報である。
type storedCredentials struct {
	password *passwordOffer
	totp     *totpOffer
}

// newStoredCredentials は、接続ごとの保存済みパスワードとTOTPの出し先を作る。
// それぞれを1度だけ出すのは、passwordOffer と totpOffer の側である。
func (a Auth) newStoredCredentials(target Target) storedCredentials {
	return storedCredentials{
		password: &passwordOffer{target: target, provider: a.Password},
		totp:     &totpOffer{target: target, provider: a.TOTP},
	}
}

// explainUnanswered は、保存済みの値が拒否されたあとで利用者に尋ねられなかった
// 失敗を、認証の拒否としても見分けられるようにする。
//
// 非対話の接続では、拒否のあとに尋ねる質問が ErrPromptUnavailable で終わる。
// そのままでは「入力が要る」としか読めず、保存した値が拒否されたことが伝わらない。
func (stored storedCredentials) explainUnanswered(err error) error {
	if !errors.Is(err, ErrPromptUnavailable) {
		return err
	}
	var rejected []string
	if stored.password.wasRejected() {
		rejected = append(rejected, "password")
	}
	if stored.totp.wasRejected() {
		rejected = append(rejected, "verification code")
	}
	if len(rejected) == 0 {
		return err
	}
	return storedCredentialRejected{credentials: rejected, err: err}
}

// Package randomid は、乱数から一覧のキーにする識別子と、認証に使うトークンを作る。
//
// 長さ・表記・引き直しの上限をここにだけ置く。使う側ごとに書くと、乱数源が壊れた
// ときの失敗が「上限に達した」のような別の理由に化ける。
package randomid

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

const (
	// idBytes は、識別子に使う乱数の長さ。128ビットあれば、一覧の中で偶然に重なる
	// ことは実際には起きない。
	idBytes = 16
	// tokenBytes は、トークンに使う乱数の長さ。256ビットあれば総当たりで当てられない。
	tokenBytes = 32
	// unusedIDAttempts は、使用中の識別子と重なったときに引き直す回数の上限。
	// 128ビットの乱数が続けて重なるのは乱数源が壊れているか固定されているときだけ
	// なので、少ない回数で打ち切る。
	unusedIDAttempts = 8
)

// ErrExhausted は、引き直しても使用中でない識別子が出なかったことを表す。
// 乱数源が同じ値を返し続けている。件数の上限に達したのではない。
var ErrExhausted = errors.New("the random source kept producing identifiers that are already in use")

// UnusedID は、inUse が使用中と答えない識別子を、16バイトの乱数の16進で返す。
func UnusedID(random io.Reader, inUse func(id string) (bool, error)) (string, error) {
	raw := make([]byte, idBytes)
	for range unusedIDAttempts {
		if _, err := io.ReadFull(random, raw); err != nil {
			return "", err
		}
		id := hex.EncodeToString(raw)
		taken, err := inUse(id)
		if err != nil {
			return "", err
		}
		if !taken {
			return id, nil
		}
	}
	return "", ErrExhausted
}

// Token は、32バイトの乱数をパディングなしの base64url で返す。
func Token(random io.Reader) (string, error) {
	return token(random, base64.RawURLEncoding.EncodeToString)
}

// IsToken は、value が Token の作る形（32バイトの乱数の base64url）かを返す。
// 長さを先に見て、長すぎる入力を復号しない。
func IsToken(value string) bool {
	if len(value) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == tokenBytes
}

// token は、読んだ乱数を返す前に消す。トークンはシークレットなので、元の乱数を
// ヒープに残さない。encode はテストが消したことを確かめるための差し替え口。
func token(random io.Reader, encode func([]byte) string) (string, error) {
	raw := make([]byte, tokenBytes)
	defer clear(raw)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", err
	}
	return encode(raw), nil
}

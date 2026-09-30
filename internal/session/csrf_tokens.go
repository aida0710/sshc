package session

import (
	"crypto/sha256"
	"crypto/subtle"
	"slices"
)

// MaxCSRFTokensPerSession bounds memory and the lifetime of an abandoned tab
// token. Past the bound, the token retired first is the one least likely to
// still be held by an open tab (see issuedCSRFTokens).
const MaxCSRFTokensPerSession = 8

// issuedCSRFTokens は、1 つのセッションが配った CSRF token の hash を、上限を
// 越えたときに退役させる順に並べる。先頭側に renew で置き換えられた token を
// 置き換えた順に、後ろ側にまだ使われている token を最後に使った順に置く。
//
// 発行順で退役させると、開いたままのタブの token と、再読み込みや閉じたタブが
// 残した使われない token を区別できない。ほかのタブを 8 回読み込んだだけで、
// 開いたままのタブが切れてしまう。
type issuedCSRFTokens []issuedCSRFToken

type issuedCSRFToken struct {
	hash [sha256.Size]byte
	// replaced は、この token を持っていたページが renew で新しい token に
	// 置き換えたことを表す。まだ持っているのは、sessionStorage を複製した
	// タブ（ブラウザの「タブを複製」）だけである。
	replaced bool
}

func newIssuedCSRFTokens(csrf string) issuedCSRFTokens {
	return issuedCSRFTokens{{hash: sha256.Sum256([]byte(csrf))}}
}

// find は presented と一致する token の位置を返す。一致しなければ -1 を返す。
// どの位置で一致しても、すべての候補を同じ時間で比べる。
func (tokens issuedCSRFTokens) find(presented string) int {
	presentedHash := sha256.Sum256([]byte(presented))
	found := -1
	for index, candidate := range tokens {
		matched := subtle.ConstantTimeCompare(presentedHash[:], candidate.hash[:])
		found = subtle.ConstantTimeSelect(matched, index, found)
	}
	return found
}

// markUsed は index の token を、最後に退役させる位置へ移す。置き換えられた
// token でも、使われたなら複製したタブが持っているので、使われている側へ戻す。
func (tokens issuedCSRFTokens) markUsed(index int) issuedCSRFTokens {
	used := tokens[index]
	used.replaced = false
	tokens = slices.Delete(tokens, index, index+1)
	return append(tokens, used)
}

// markReplaced は index の token を、置き換えられた token の最後へ移す。
// 使われている token より先に、ただし先に置き換えられた token よりは後に退役する。
func (tokens issuedCSRFTokens) markReplaced(index int) issuedCSRFTokens {
	replaced := tokens[index]
	replaced.replaced = true
	tokens = slices.Delete(tokens, index, index+1)
	firstInUse := slices.IndexFunc(tokens, func(token issuedCSRFToken) bool { return !token.replaced })
	if firstInUse < 0 {
		firstInUse = len(tokens)
	}
	return slices.Insert(tokens, firstInUse, replaced)
}

// add は新しい token を最後に退役させる位置へ足す。上限に達していれば、
// 先頭から退役させて場所を空ける。
func (tokens issuedCSRFTokens) add(csrf string) issuedCSRFTokens {
	if overflow := len(tokens) - MaxCSRFTokensPerSession + 1; overflow > 0 {
		tokens = slices.Delete(tokens, 0, overflow)
	}
	return append(tokens, issuedCSRFToken{hash: sha256.Sum256([]byte(csrf))})
}

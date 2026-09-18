package handoff

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"io"
)

// CLI は最初の要求で handoff の秘密を送る前に、相手が本当にその秘密を持つ engine か
// を確かめる。engine が止まったあとに handoff だけが残り、同じ port を別の process
// が取っていると、CLI は master password や S3 の資格情報を偽の engine へ渡して
// しまう。challenge は CLI が毎回作る乱数で、engine は秘密で HMAC した proof を返す。
// 秘密を知らない process は proof を作れず、proof から秘密を逆算することもできない。
const (
	// ChallengeHeader は、CLI が engine に証明を求める乱数を運ぶ。
	ChallengeHeader = "X-SSHC-CLI-Challenge"
	// ProofHeader は、engine が返す HMAC を運ぶ。
	ProofHeader = "X-SSHC-CLI-Proof"
	// proofLabel は、同じ秘密で作る他の値と proof が衝突しないよう message に前置する。
	proofLabel = "sshc engine proof\x00"
)

// MintChallenge は、1 回の確認に使う乱数を返す。
func MintChallenge(random io.Reader) (string, error) {
	return mint(random, base64.RawURLEncoding.EncodeToString)
}

// ValidChallenge は、challenge がこのパッケージの作った形かを返す。
func ValidChallenge(challenge string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(challenge)
	return err == nil && len(decoded) == secretLength
}

// Prove は、secret を持つことを challenge に対して示す値を返す。
func Prove(secret, challenge string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(proofLabel + challenge))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// VerifyProof は、proof が secret と challenge から作られたものかを定数時間で返す。
func VerifyProof(secret, challenge, proof string) bool {
	expected := Prove(secret, challenge)
	return len(proof) == len(expected) && subtle.ConstantTimeCompare([]byte(proof), []byte(expected)) == 1
}

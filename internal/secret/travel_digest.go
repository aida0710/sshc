package secret

import (
	"encoding/hex"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

// travelDigestPurpose は、同期状態に残す vault 文書の digest を鍵付きにするときの
// HKDF の用途名。vault の暗号化鍵そのものを MAC に使い回さないために分ける。
const travelDigestPurpose = "sshc sync-state vault document digest"

// KeyedTravelDigest は、同期状態のファイルに残す vault 文書の digest を、この
// マシンの vault の鍵で鍵付きにする。素の SHA-256 を残すと、~/.ssh の写しを
// 手に入れた者が、マスターパスワードの Argon2id を経ずに保存済みのシークレットを
// 総当たりで照合できる。
//
// 鍵はマスターパスワードの世代ごとに変わる。鍵を変える変更は、同じトランザクション
// で同期状態の値を新しい鍵へ移す（TravelDigestRekey）。
func (s *Service) KeyedTravelDigest(digest string) (string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	vault := s.use()
	if vault == nil {
		return "", ErrLocked
	}
	return keyedTravelDigest(vault.key, digest)
}

func keyedTravelDigest(key envelope.Key, digest string) (string, error) {
	sum, err := key.MAC(travelDigestPurpose, []byte(digest))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum), nil
}

// TravelDigestRekey は、vault の鍵が変わる変更の中で、同期状態に鍵付きで残した
// vault 文書の digest を新しい鍵へ移すための材料である。鍵そのものは渡さない。
type TravelDigestRekey struct {
	// Digest は、変更のあとの vault 文書の素の digest。同期が集めるときと同じく、
	// 空の vault は空の文書の digest になる。
	Digest string
	// Previous は、変更の前の鍵で素の digest を鍵付きにする。
	Previous func(digest string) (string, error)
	// Next は、変更のあとの鍵で素の digest を鍵付きにする。
	Next func(digest string) (string, error)
}

// SetTravelDigestRekey は、vault の鍵が変わる変更（マスターパスワードの変更、
// 復旧、reset）に載せる、同期状態の書き換えを作る関数を取り付ける。同期状態は
// 同期のパッケージのファイルなので、中身はそちらが決める。
//
// rekey は変更の組み立ての中で、s.mutex を持ったまま呼ぶことがある。この Service を
// 呼び返してはならない。
func (s *Service) SetTravelDigestRekey(rekey func(TravelDigestRekey) ([]storage.Change, error)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.travelDigestRekey = rekey
}

// travelDigestRekeyChanges は、vault の鍵が previous から candidate の鍵へ変わる
// 変更に載せる、同期状態の書き換えを返す。
func (s *Service) travelDigestRekeyChanges(previous envelope.Key, candidate *Vault) ([]storage.Change, error) {
	if s.travelDigestRekey == nil {
		return nil, nil
	}
	var document []byte
	if !candidate.Empty() {
		var err error
		if document, err = candidate.Document(); err != nil {
			return nil, err
		}
	}
	digest := storage.Digest(document)
	clear(document)
	return s.travelDigestRekey(TravelDigestRekey{
		Digest:   digest,
		Previous: func(digest string) (string, error) { return keyedTravelDigest(previous, digest) },
		Next:     func(digest string) (string, error) { return keyedTravelDigest(candidate.key, digest) },
	})
}

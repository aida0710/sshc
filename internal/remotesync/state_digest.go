package remotesync

import (
	"slices"
	"strings"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// keyedDigestPrefix は、sync-state.json の Base に残した vault 文書（TravelPath）の
// digest が、このマシンの vault の鍵で鍵付きにした値であることを示す。
//
// vault 文書の素の SHA-256 をディスクに残すと、~/.ssh の写しを手に入れた者が、
// マスターパスワードの Argon2id を経ずに保存済みのシークレットを総当たりで照合
// できる。リモートのマニフェストは同期キーの envelope の内側にあるので素のまま
// でよく、鍵付きにするのはこのマシンの state に書くときだけである。
const keyedDigestPrefix = "hmac-sha256:"

// travelEntryIndex は、manifest の中の TravelPath の位置を返す。なければ -1。
func travelEntryIndex(manifest *Manifest) int {
	if manifest == nil {
		return -1
	}
	return slices.IndexFunc(manifest.Files, func(entry Entry) bool { return entry.Path == TravelPath })
}

// keyedTravelDigest は、素の digest を state に書く鍵付きの形にする。
// すでに鍵付きなら、そのまま返す。
func (s *Service) keyedTravelDigest(digest string) (string, error) {
	if strings.HasPrefix(digest, keyedDigestPrefix) {
		return digest, nil
	}
	keyed, err := s.integrations.KeyedTravelDigest(digest)
	if err != nil {
		return "", err
	}
	return keyedDigestPrefix + keyed, nil
}

// withKeyedTravelDigest は、state に書くために、TravelPath の digest を鍵付きに
// した base の写しを返す。呼び出し側の manifest は書き換えない。
func (s *Service) withKeyedTravelDigest(base *Manifest) (*Manifest, error) {
	index := travelEntryIndex(base)
	if index < 0 {
		return base, nil
	}
	keyed, err := s.keyedTravelDigest(base.Files[index].SHA256)
	if err != nil {
		return nil, err
	}
	copied := *base
	copied.Files = slices.Clone(base.Files)
	copied.Files[index].SHA256 = keyed
	return &copied, nil
}

// comparableBase は、state から読んだ base の鍵付きの TravelPath の digest を、
// candidates（このマシンの vault 文書やリモートのマニフェストが持つ素の digest）の
// うち一致するものに戻した写しを返す。同期の三方比較は digest が等しいかだけを
// 見るので、一致したものに戻せば比較の意味は変わらない。どれとも一致しなければ
// 鍵付きのまま残し、どの素の digest とも等しくならないので「変わった」と扱われる。
func (s *Service) comparableBase(base *Manifest, candidates ...string) (*Manifest, error) {
	index := travelEntryIndex(base)
	if index < 0 {
		return base, nil
	}
	stored := base.Files[index].SHA256
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		keyed, err := s.keyedTravelDigest(candidate)
		if err != nil {
			return nil, err
		}
		if keyed != stored {
			continue
		}
		copied := *base
		copied.Files = slices.Clone(base.Files)
		copied.Files[index].SHA256 = candidate
		return &copied, nil
	}
	return base, nil
}

// travelDigest は、manifest の TravelPath の digest を返す。なければ空文字列。
func travelDigest(manifest *Manifest) string {
	index := travelEntryIndex(manifest)
	if index < 0 {
		return ""
	}
	return manifest.Files[index].SHA256
}

// RekeyTravelDigest は、vault の鍵が変わる変更（secret.SetTravelDigestRekey）に載せる
// state の書き換えを返す。state の TravelPath の digest が、変更のあとの vault 文書を
// 前の鍵で鍵付きにした値と一致するときだけ、新しい鍵で鍵付きにした値に書き換える。
// 一致しなければ、この vault は前回の同期から変わっているので、書き換えずに
// 「変わった」扱いのまま残す。
//
// vault の変更のロックの中で呼ばれるので、vault を呼び返さない。
func (s *Service) RekeyTravelDigest(rekey secret.TravelDigestRekey) ([]storage.Change, error) {
	current, err := s.readState()
	if err != nil {
		return nil, err
	}
	index := travelEntryIndex(current.Base)
	if index < 0 {
		return nil, nil
	}
	previous, err := rekey.Previous(rekey.Digest)
	if err != nil {
		return nil, err
	}
	if current.Base.Files[index].SHA256 != keyedDigestPrefix+previous {
		return nil, nil
	}
	next, err := rekey.Next(rekey.Digest)
	if err != nil {
		return nil, err
	}
	base := *current.Base
	base.Files = slices.Clone(current.Base.Files)
	base.Files[index].SHA256 = keyedDigestPrefix + next
	current.Base = &base
	change, err := s.stateChange(current)
	if err != nil {
		return nil, err
	}
	return []storage.Change{change}, nil
}

// MigrateTravelDigest は、鍵付きにする前の版が書いた state の素の TravelPath の
// digest を、今の vault の鍵で鍵付きにした値へ書き換える。素の値を鍵付きへ直すのは
// この移行だけで、comparableBase は state の値が鍵付きであることを前提にする。
// 鍵が要るので、vault を開いたあとに呼ぶ。閉じていれば secret.ErrLocked を返す。
//
// vault の変更のロックの中で書くので、マスターパスワードの変更と入れ違って
// 古い鍵の値を書くことはない。
func (s *Service) MigrateTravelDigest() error {
	return s.integrations.SecretMutation(func() error {
		current, err := s.readState()
		if err != nil {
			return err
		}
		index := travelEntryIndex(current.Base)
		if index < 0 || strings.HasPrefix(current.Base.Files[index].SHA256, keyedDigestPrefix) {
			return nil
		}
		return s.writeState(current)
	})
}

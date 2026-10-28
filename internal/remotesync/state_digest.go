package remotesync

import (
	"path/filepath"
	"slices"
	"strings"

	"sshc/internal/secret"
	"sshc/internal/storage"
)

// keyedDigestPrefix は、sync-state.json の Base に残した文書の digest が、この
// マシンの vault の鍵で鍵付きにした値であることを示す。
//
// シークレットを含む文書の素の SHA-256 をディスクに残すと、~/.ssh の写しを手に
// 入れた者が、マスターパスワードの Argon2id を経ずに保存済みのシークレットを
// 総当たりで照合できる。リモートのマニフェストは同期キーの envelope の内側に
// あるので素のままでよく、鍵付きにするのはこのマシンの state に書くときだけである。
const keyedDigestPrefix = "hmac-sha256:"

// keyedPaths は、state に digest を鍵付きで残す文書。vault 文書と、起動スニペットの
// secret 入力の値を含むスニペットの文書である。
var keyedPaths = []string{TravelPath, SnippetsPath}

// isKeyedPath は、path の digest を state に鍵付きで残すかを返す。
func isKeyedPath(path string) bool {
	return slices.Contains(keyedPaths, path)
}

// plainDigests は、鍵付きにする path ごとの、state の値と比べる素の digest の候補。
type plainDigests map[string][]string

// addManifest は、manifest が持つ鍵付きにする path の digest を候補に加える。
func (candidates plainDigests) addManifest(manifest *Manifest) plainDigests {
	if manifest == nil {
		return candidates
	}
	for _, entry := range manifest.Files {
		if isKeyedPath(entry.Path) {
			candidates[entry.Path] = append(candidates[entry.Path], entry.SHA256)
		}
	}
	return candidates
}

// addLocal は、このマシンの文書の digest を候補に加える。
func (candidates plainDigests) addLocal(local map[string]LocalEntry) plainDigests {
	for _, path := range keyedPaths {
		if entry, ok := local[path]; ok {
			candidates[path] = append(candidates[path], entry.SHA256)
		}
	}
	return candidates
}

// keyedDigest は、素の digest を state に書く鍵付きの形にする。
// すでに鍵付きなら、そのまま返す。
func (s *Service) keyedDigest(digest string) (string, error) {
	if strings.HasPrefix(digest, keyedDigestPrefix) {
		return digest, nil
	}
	keyed, err := s.integrations.KeyedTravelDigest(digest)
	if err != nil {
		return "", err
	}
	return keyedDigestPrefix + keyed, nil
}

// withKeyedDigests は、state に書くために、鍵付きにする path の digest を鍵付きに
// した base の写しを返す。呼び出し側の manifest は書き換えない。
func (s *Service) withKeyedDigests(base *Manifest) (*Manifest, error) {
	if base == nil {
		return nil, nil
	}
	var copied *Manifest
	for index, entry := range base.Files {
		if !isKeyedPath(entry.Path) {
			continue
		}
		keyed, err := s.keyedDigest(entry.SHA256)
		if err != nil {
			return nil, err
		}
		if keyed == entry.SHA256 {
			continue
		}
		if copied == nil {
			copied = cloneManifest(base)
		}
		copied.Files[index].SHA256 = keyed
	}
	if copied == nil {
		return base, nil
	}
	return copied, nil
}

// comparableBase は、state から読んだ base の鍵付きの digest を、candidates（この
// マシンの文書やリモートのマニフェストが持つ素の digest）のうち一致するものに戻した
// 写しを返す。同期の三方比較は digest が等しいかだけを見るので、一致したものに戻せば
// 比較の意味は変わらない。どれとも一致しなければ鍵付きのまま残し、どの素の digest
// とも等しくならないので「変わった」と扱われる。鍵付きにする前の版が書いた素の値は、
// 素の digest とそのまま比べられるので、手を付けずに残す。
func (s *Service) comparableBase(base *Manifest, candidates plainDigests) (*Manifest, error) {
	if base == nil {
		return nil, nil
	}
	var copied *Manifest
	for index, entry := range base.Files {
		if !isKeyedPath(entry.Path) || !strings.HasPrefix(entry.SHA256, keyedDigestPrefix) {
			continue
		}
		plain, err := s.matchingPlainDigest(entry.SHA256, candidates[entry.Path])
		if err != nil {
			return nil, err
		}
		if plain == "" {
			continue
		}
		if copied == nil {
			copied = cloneManifest(base)
		}
		copied.Files[index].SHA256 = plain
	}
	if copied == nil {
		return base, nil
	}
	return copied, nil
}

// matchingPlainDigest は、鍵付きにすると stored になる候補を返す。なければ空文字列。
func (s *Service) matchingPlainDigest(stored string, candidates []string) (string, error) {
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		keyed, err := s.keyedDigest(candidate)
		if err != nil {
			return "", err
		}
		if keyed == stored {
			return candidate, nil
		}
	}
	return "", nil
}

// cloneManifest は、Files を書き換えられる manifest の写しを返す。
func cloneManifest(manifest *Manifest) *Manifest {
	copied := *manifest
	copied.Files = slices.Clone(manifest.Files)
	return &copied
}

// RekeyTravelDigest は、vault の鍵が変わる変更（secret.SetTravelDigestRekey）に載せる
// state の書き換えを返す。state の鍵付きの digest が、変更のあとの文書を前の鍵で
// 鍵付きにした値と一致するときだけ、新しい鍵で鍵付きにした値に書き換える。
// 一致しなければ、その文書は前回の同期から変わっているので、書き換えずに
// 「変わった」扱いのまま残す。
//
// vault の変更のロックの中で呼ばれるので、vault を呼び返さない。
func (s *Service) RekeyTravelDigest(rekey secret.TravelDigestRekey) ([]storage.Change, error) {
	current, err := s.readState()
	if err != nil {
		return nil, err
	}
	if current.Base == nil {
		return nil, nil
	}
	var rekeyed *Manifest
	for index, entry := range current.Base.Files {
		plain, ok := s.plainDigestAfterRekey(entry.Path, rekey)
		if !ok {
			continue
		}
		previous, err := rekey.Previous(plain)
		if err != nil {
			return nil, err
		}
		if entry.SHA256 != keyedDigestPrefix+previous {
			continue
		}
		next, err := rekey.Next(plain)
		if err != nil {
			return nil, err
		}
		if rekeyed == nil {
			rekeyed = cloneManifest(current.Base)
		}
		rekeyed.Files[index].SHA256 = keyedDigestPrefix + next
	}
	if rekeyed == nil {
		return nil, nil
	}
	current.Base = rekeyed
	change, err := s.stateChange(current)
	if err != nil {
		return nil, err
	}
	return []storage.Change{change}, nil
}

// plainDigestAfterRekey は、鍵を変える変更のあとの、path の文書の素の digest を返す。
// スニペットの文書は、ファイルの平文がそのまま同期で運ぶ文書になる。
func (s *Service) plainDigestAfterRekey(path string, rekey secret.TravelDigestRekey) (string, bool) {
	switch path {
	case TravelPath:
		return rekey.Digest, true
	case SnippetsPath:
		digest, ok := rekey.ProtectedDigests[filepath.Join(s.workspace.Root(), filepath.FromSlash(SnippetsPath))]
		return digest, ok
	default:
		return "", false
	}
}

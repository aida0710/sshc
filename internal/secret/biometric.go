package secret

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

// BiometricPath は、生体認証で開けるための二つ目の入口。
//
// **中身は保管庫の写しではない。** 保管庫の鍵を、OS の錠前が預かっている秘密の下に
// 封じたものである。錠前が開かなければ、このファイルは何の役にも立たない。
const BiometricPath = "sshc/secrets.biometric"

var (
	// ErrNoGuardian は、この端末に預かってくれる錠前が無いことを報告する。
	ErrNoGuardian = errors.New("this machine has no lock to keep the secret in")
	// ErrNoBiometric は、この端末でまだ預けていないことを報告する。
	ErrNoBiometric = errors.New("nothing has been kept for this machine")
	// ErrRefused は、人が断ったか、生体が通らなかったことを報告する。**失敗では
	// ない。** パスワードの道は開いたままである。
	ErrRefused = errors.New("the person did not prove themselves")
)

// Guardian は、OS の錠前である。
//
// **鍵そのものは知らない。** 預かるのは、この端末でだけ意味を持つ秘密であり、
// それを返す条件——指紋、顔、PIN——を決めるのは OS であってこちらではない。
//
// この interface が in-process に在るのは、macOS では engine 自身が Keychain を
// 叩けるからである。窓を持っている側にしか出せないプロンプト（Windows Hello）は、
// 外殻が同じ形の口を通して渡す。
type Guardian interface {
	// Available は、この端末にその錠前があるかを答える。**尋ねるだけで、
	// 何も出さない。** 画面がトグルを見せるかどうかがこれで決まる。
	Available() bool
	// Keep は預ける。以後、取り出すには本人であることの証明が要る。
	Keep(secret []byte) error
	// Reveal は、証明を通してから返す。人が断れば ErrRefused。
	Reveal() ([]byte, error)
	// Forget は預かりを解く。預かっていなくても成功する。
	Forget() error
}

// keptSecretBytes は、預ける秘密の長さ。160 ビット。
//
// **人が覚えるものではない。** 読むのも打つのも OS なので、長さは強さだけで決めて
// よい。base32 にしてから渡すのは、Keychain も Keystore も文字列を持つ方が素直で
// あり、`envelope.Derive` の入口が文字列だからである。
const keptSecretBytes = 20

var keptEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// BiometricState は、画面が知ってよいことのすべてである。
type BiometricState struct {
	// Available は、この端末に錠前があるか。
	Available bool `json:"available"`
	// Enabled は、この端末で預けてあるか。
	Enabled bool `json:"enabled"`
}

// Biometric は、いまの状態を返す。**値は何も返さない。**
func (s *Service) Biometric() BiometricState {
	state := BiometricState{Available: s.guardian != nil && s.guardian.Available()}
	if !state.Available {
		return state
	}
	_, err := s.workspace.FileSystem().ReadFile(s.biometricPath())
	state.Enabled = err == nil
	return state
}

// SetGuardian は、この実行の錠前を差す。nil はこの端末に錠前が無いことである。
func (s *Service) SetGuardian(guardian Guardian) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guardian = guardian
}

func (s *Service) biometricPath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(BiometricPath))
}

// EnableBiometric は、この端末の錠前に二つ目の入口を預ける。
//
// **保管庫が開いていることが条件である。** 開いていない状態から預けられるなら、
// それは「パスワードを知らなくても入口を作れる」ということになる。
func (s *Service) EnableBiometric() error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	guardian := s.guardian
	vault := s.use()
	s.mu.Unlock()
	if guardian == nil || !guardian.Available() {
		return ErrNoGuardian
	}
	if vault == nil {
		return ErrLocked
	}

	raw := make([]byte, keptSecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	kept := keptEncoding.EncodeToString(raw)
	under, err := envelope.Derive(kept)
	if err != nil {
		return err
	}
	wrapped, err := vault.Wrap(under)
	if err != nil {
		return err
	}

	// **先に書き、それから預ける。** 逆にすると、書き込みに失敗した端末の錠前に、
	// 何も開けない秘密が residue として残る。
	if err := s.writeBiometric(wrapped); err != nil {
		return err
	}
	if err := guardian.Keep([]byte(kept)); err != nil {
		// 預けられなかったなら、開けられない入口を残さない。
		_ = s.removeBiometric()
		return err
	}
	return nil
}

// UnlockWithBiometric は、錠前に本人を確かめさせてから保管庫を開ける。
func (s *Service) UnlockWithBiometric() error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	guardian := s.guardian
	s.mu.Unlock()
	if guardian == nil || !guardian.Available() {
		return ErrNoGuardian
	}
	wrapped, err := s.workspace.FileSystem().ReadFile(s.biometricPath())
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNoBiometric
	}
	if err != nil {
		return err
	}
	sealed, err := s.workspace.FileSystem().ReadFile(s.path())
	if errors.Is(err, fs.ErrNotExist) {
		return ErrNoVault
	}
	if err != nil {
		return err
	}

	// ここで OS のプロンプトが出る。断られたら、そこで終わりである。
	kept, err := guardian.Reveal()
	if err != nil {
		return err
	}
	key, err := envelope.Unwrap(wrapped, strings.TrimSpace(string(kept)))
	if err != nil {
		// **預かりと保管庫が食い違っている。** マスターパスワードを変えたあとに
		// 残っていた、といった形である。捨てて、パスワードの道へ返す。
		_ = s.ForgetBiometric()
		return ErrNoBiometric
	}
	vault, err := OpenWith(sealed, key)
	if err != nil {
		_ = s.ForgetBiometric()
		return ErrNoBiometric
	}

	s.mu.Lock()
	s.vault = vault
	s.baseline = append([]byte(nil), sealed...)
	s.used = s.now()
	s.refusals = 0
	s.mu.Unlock()
	return nil
}

// ForgetBiometric は、この端末の預かりを解く。
//
// **片方だけ消えても成功にする。** 残るのは「開けない入口」か「何も開けない秘密」
// のどちらかで、どちらも次の有効化が上書きする。
func (s *Service) ForgetBiometric() error {
	s.mu.Lock()
	guardian := s.guardian
	s.mu.Unlock()
	removed := s.removeBiometric()
	if guardian == nil {
		return removed
	}
	if err := guardian.Forget(); err != nil {
		return err
	}
	return removed
}

func (s *Service) writeBiometric(wrapped []byte) error {
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	current, readErr := s.workspace.FileSystem().ReadFile(s.biometricPath())
	precondition := storage.Precondition{}
	if readErr == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	}
	_, err := s.transactions.Commit(storage.Request{
		Operation: "vault.biometric",
		Changes: []storage.Change{{
			Path: s.biometricPath(), Contents: wrapped, Precondition: precondition,
			// **世代を残さない。** 古い入口は古い秘密の下に封じられており、
			// その秘密はもう錠前の中に無い。開けないものを積み上げるだけである。
			SkipBackup: true,
		}},
	})
	return err
}

func (s *Service) removeBiometric() error {
	current, err := s.workspace.FileSystem().ReadFile(s.biometricPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "vault.biometric.forget",
		Removals: []storage.Removal{{
			Path:         s.biometricPath(),
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(current)},
		}},
	})
	return err
}

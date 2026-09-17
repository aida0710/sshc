package secret

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

// IdleTimeout は、いま設定されている時計を返す。配線が効いていることを、
// 呼び出し側の外から確かめられるようにするためにある。
func (s *Service) IdleTimeout() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idle
}

// SetIdleTimeout は、これ以降に使う自動ロック時間を変更する。
// 0は自動ロックなしであり、手動Lockとengine終了による破棄は変えない。
func (s *Service) SetIdleTimeout(timeout time.Duration) {
	if timeout < 0 {
		panic("secret: negative idle timeout")
	}
	s.mu.Lock()
	s.idle = timeout
	// すでに新しい期限を超えていれば、次のrequestを待たずに破棄する。
	s.open()
	s.mu.Unlock()
}

// open は vault を返す。IdleTimeout より長く触れられていなければ、先にそれを
// 閉じる。
//
// 秘密に触れるすべてのメソッドは、s.vault 自体を調べるのではなくここを通る。
// そのため、vault が開いているかを判断する場所はひとつだけになり、それを尋ね
// 忘れたメソッドを書くことはできない。
func (s *Service) open() *Vault {
	if s.vault == nil {
		return nil
	}
	if !s.passwordless && s.idle > 0 && s.now().Sub(s.used) >= s.idle {
		s.vault.Destroy()
		s.vault = nil
		s.baseline = nil
		s.lastMigration = Migration{}
		return nil
	}
	return s.vault
}

// use は vault を返し、アイドルの時計をゼロに戻す。これは、秘密があるかどうかを
// 報告するのではなく、まさに秘密を読み書きしようとするときにメソッドが呼ぶもので
// ある。
func (s *Service) use() *Vault {
	vault := s.open()
	if vault != nil && s.unattended == 0 {
		s.used = s.now()
	}
	return vault
}

// Unattended は、自動処理による参照を vault の利用時刻に数えないようにする。
// 期限を過ぎている場合は open が通常どおりロックする。
func (s *Service) Unattended(run func()) {
	s.mu.Lock()
	s.unattended++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.unattended--
		s.mu.Unlock()
	}()
	run()
}

func (s *Service) path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(WorkspacePath))
}

// Exists は vault ファイルが存在するかを報告する。これは、それがロック解除されて
// いるかという問いとは別のものである。
//
// 中身は読まない。有るか無いかを尋ねているだけであり、結果はファイルの
// 存在そのものにある。ここが `ReadFile` だったころは、メニューバーを開くたびに
// vault 全体（暗号文とはいえ、保存された結果の全部）が読み込まれてプロセスの
// メモリを通っていた。
func (s *Service) Exists() (bool, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	return s.exists()
}

// exists は mutationMu をすでに保持する operation が存在だけを読む。
func (s *Service) exists() (bool, error) {
	_, err := s.workspace.FileSystem().Lstat(s.path())
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// State は disk 上の存在と memory 上のロック解除状態を同じ mutation 境界で読む。
// status polling は秘密を使わないため、idle deadline は更新しない。
func (s *Service) State() (State, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	exists, err := s.exists()
	if err != nil {
		return State{}, err
	}
	localKey, err := s.localKey()
	if err != nil {
		return State{}, err
	}
	passwordless := len(localKey) > 0
	clear(localKey)
	s.mu.Lock()
	if !exists {
		// disk 上の vault を失ったあとも導出済み key だけを使い続けない。
		s.vault.Destroy()
		s.vault = nil
		s.baseline = nil
		s.lastMigration = Migration{}
	}
	unlocked := s.open() != nil
	migration := s.lastMigration
	s.mu.Unlock()
	return State{Exists: exists, Unlocked: unlocked, LastMigration: migration, Passwordless: passwordless}, nil
}

// Unlocked は、このセッションでパスフレーズが与えられたかを報告する。
func (s *Service) Unlocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.open() != nil
}

// Initialise は、vault を持たないワークスペースのために vault を作る。
//
// すでに存在する場合は置き換えずに拒否する。うっかり再初期化すれば、保存済みの
// パスワードがすべて破壊されるし、鍵が失われた暗号化ファイルに復旧の道は
// ない。
func (s *Service) Initialise(passphrase string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	exists, err := s.exists()
	if err != nil {
		return err
	}
	if exists {
		return ErrAlreadyExists
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	localChange, effective, err := s.prepareProtection(passphrase)
	if err != nil {
		return err
	}
	defer clear(localChange.Contents)
	vault, err := Create(effective)
	if err != nil {
		return err
	}
	sealed, err := vault.Seal()
	if err != nil {
		vault.Destroy()
		return err
	}
	_, err = s.transactions.CommitAtomicDiscardBackups(storage.Request{
		Operation: "secret.vault",
		Changes: []storage.Change{{
			Path: s.path(), Contents: sealed,
			// A zero precondition means the path must still be absent. Another
			// initializer which wins after exists() must never be overwritten.
			Precondition: storage.Precondition{},
		}, localChange},
	})
	if err != nil {
		vault.Destroy()
		return err
	}

	// Publish only the exact candidate which is now durable. Readers continue
	// to observe a locked service while encryption and storage are in flight.
	s.mu.Lock()
	s.vault.Destroy()
	s.vault = vault
	s.passwordless = passphrase == ""
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.lastMigration = Migration{}
	s.mu.Unlock()
	return nil
}

// Verify は、passphrase がこのワークスペースのマスターパスワードかを報告する。
//
// ファイルから判定し、何も変えない。したがって閉じた vault にも尋ねられるし、画面は、
// ユーザーが打ち込んだものをマスターパスワードとして使う前に、それがマスター
// パスワードかどうかを知ることができる。スナップショットを二つ目のパスワードでは
// なくマスターパスワードで暗号化されるのはこれのおかげだ。打ち間違いは、誰にも開け
// ないアーカイブではなく、ここでの拒否になる。
//
// コストは導出 1 回分で、ロック解除と同じである。しかもここに到達するのは、ユーザーが
// 求めた操作からだけだ。
func (s *Service) Verify(passphrase string) (bool, error) {
	passphrase, err := s.resolvePassphrase(passphrase)
	if err != nil {
		return false, err
	}
	sealed, err := s.workspace.FileSystem().ReadFile(s.path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, ErrNoVault
		}
		return false, err
	}
	vault, err := Open(sealed, passphrase)
	if err != nil {
		if errors.Is(err, ErrWrongPassphrase) {
			return false, nil
		}
		return false, err
	}
	vault.Destroy()
	return true, nil
}

// MaxUnlockDelay は、拒否が待つ最長時間。
//
// vault ファイルはコピーしてオフラインで攻撃できるので、これは攻撃者とその中身の
// あいだに立つものではない、それは Argon2id である。これが止めるのは安価な場合だ。
// すなわち、動作中のアプリケーションに対して、判定できる限りの速さでパスワードを
// 試すローカルのプロセスである。
const MaxUnlockDelay = 4 * time.Second

// SetSleep は、拒否がどう待つかを取り付ける。バックオフを消費せずに観測するテスト
// のためのものである。
func (s *Service) SetSleep(sleep func(time.Duration)) { s.sleep = sleep }

// refuse は、この実行での連続した拒否が招いた分だけ待つ。
func (s *Service) refuse() {
	s.mu.Lock()
	s.refusals++
	count := s.refusals
	s.mu.Unlock()

	delay := time.Duration(count) * 250 * time.Millisecond
	if delay > MaxUnlockDelay {
		delay = MaxUnlockDelay
	}
	sleep := s.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	sleep(delay)
}

// Unlock は passphrase で vault を開く。
func (s *Service) Unlock(passphrase string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	passwordless := passphrase == ""
	passphrase, err := s.resolvePassphrase(passphrase)
	if err != nil {
		return err
	}
	sealed, err := s.workspace.FileSystem().ReadFile(s.path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNoVault
		}
		return err
	}
	vault, migration, err := openSealedWithMigrations(sealed, passphrase, s.migrations)
	if err != nil {
		if errors.Is(err, ErrWrongPassphrase) {
			s.refuse()
		}
		return err
	}
	s.mu.Lock()
	s.passwordless = passwordless
	s.mu.Unlock()
	if migration.Applied() {
		migrated, sealErr := vault.Seal()
		if sealErr != nil {
			vault.Destroy()
			return &MigrationError{From: migration.From, To: migration.To, Cause: sealErr}
		}
		return s.replaceVault(sealed, migrated, vault, nil, nil, "secret.migrate-vault", migration)
	}

	s.mu.Lock()
	if s.vault != nil && s.vault != vault {
		s.vault.Destroy()
	}
	s.vault = vault
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.lastMigration = Migration{}
	// 通ったパスワードは、誤ったものが積み上げたものを消し去る。
	s.refusals = 0
	s.mu.Unlock()
	return nil
}

// RecoverCompatibleBackup は、現在のvaultが復号後のschema不一致だった場合に限り、
// 同じmaster passwordで開ける直近の現行schema世代へ戻す。世代backupはvault鍵で
// 二重に封じられているが、envelope headerがsaltを運ぶためpassphraseから直接開ける。
func (s *Service) RecoverCompatibleBackup(passphrase string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	passphrase, err := s.resolvePassphrase(passphrase)
	if err != nil {
		return err
	}

	current, err := s.unsupportedVault(passphrase)
	if err != nil {
		return err
	}
	history, err := s.transactions.History()
	if err != nil {
		return err
	}

	const maximumCandidates = 32
	checked := 0
	for _, record := range history {
		if checked >= maximumCandidates || !slices.Contains(record.Paths, s.path()) {
			continue
		}
		checked++
		backupPath := filepath.Join(record.BackupDir, filepath.FromSlash(WorkspacePath))
		wrapped, readErr := s.workspace.FileSystem().ReadFile(backupPath)
		if errors.Is(readErr, fs.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return readErr
		}
		candidateSealed, backupKey, openErr := envelope.Open(wrapped, passphrase)
		if openErr != nil {
			continue
		}
		backupKey.Destroy()
		candidate, migration, openErr := openSealedWithMigrations(candidateSealed, passphrase, s.migrations)
		if openErr != nil {
			continue
		}
		if migration.Applied() {
			candidateSealed, openErr = candidate.Seal()
			if openErr != nil {
				candidate.Destroy()
				return &MigrationError{From: migration.From, To: migration.To, Cause: openErr}
			}
		}
		_, previous, keyErr := envelope.Open(current, passphrase)
		if keyErr != nil {
			candidate.Destroy()
			return keyErr
		}
		documents, reSealErr := s.reSealKeyBoundArtifacts(candidate, previous, passphrase, true, true)
		previous.Destroy()
		if reSealErr != nil {
			candidate.Destroy()
			return reSealErr
		}
		return s.replaceVault(current, candidateSealed, candidate, documents, nil, "secret.recover-compatible-backup", migration)
	}
	return ErrNoCompatibleBackup
}

// ResetUnsupported は、正しいmaster passwordで復号できるがschemaだけが扱えない
// vaultを空の現行vaultへ置き換える。SSH設定と鍵は変更せず、同期資格情報は古いvault
// 鍵へ結び付いているため同じtransactionで外す。同期設定は明示的なreset契約に従い、
// 開けない旧世代の内側暗号文をbackupにも残さない。
func (s *Service) ResetUnsupported(passphrase string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	passphrase, err := s.resolvePassphrase(passphrase)
	if err != nil {
		return err
	}

	current, err := s.unsupportedVault(passphrase)
	if err != nil {
		return err
	}
	candidate, err := Create(passphrase)
	if err != nil {
		return err
	}
	sealed, err := candidate.Seal()
	if err != nil {
		candidate.Destroy()
		return err
	}
	var removals []storage.Removal
	settingsPath := s.settingsPath()
	settings, readErr := s.workspace.FileSystem().ReadFile(settingsPath)
	switch {
	case readErr == nil:
		// Reset intentionally forgets synchronization credentials. Do not retain
		// an old-generation nested ciphertext which generic history restore could
		// write back but the candidate key could not open.
		removals = append(removals, storage.Removal{
			Path: settingsPath, Backup: false,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(settings)},
		})
	case errors.Is(readErr, fs.ErrNotExist):
	default:
		candidate.Destroy()
		return readErr
	}

	_, previous, err := envelope.Open(current, passphrase)
	if err != nil {
		candidate.Destroy()
		return err
	}
	documents, err := s.reSealKeyBoundArtifacts(candidate, previous, passphrase, true, false)
	previous.Destroy()
	if err != nil {
		candidate.Destroy()
		return err
	}
	return s.replaceVault(current, sealed, candidate, documents, removals, "secret.reset-unsupported", Migration{})
}

func (s *Service) unsupportedVault(passphrase string) ([]byte, error) {
	sealed, err := s.workspace.FileSystem().ReadFile(s.path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoVault
		}
		return nil, err
	}
	opened, _, err := openSealedWithMigrations(sealed, passphrase, s.migrations)
	switch {
	case err == nil:
		opened.Destroy()
		return nil, ErrRecoveryNotNeeded
	case errors.Is(err, ErrWrongPassphrase):
		s.refuse()
		return nil, err
	case errors.Is(err, ErrOlderSchema), errors.Is(err, ErrNewerSchema):
		return sealed, nil
	default:
		return nil, err
	}
}

func (s *Service) replaceVault(
	current []byte,
	sealed []byte,
	candidate *Vault,
	changes []storage.Change,
	removals []storage.Removal,
	operation string,
	migration Migration,
) error {
	localKey, err := s.localKey()
	if err != nil {
		candidate.Destroy()
		return err
	}
	passwordless := len(localKey) > 0
	clear(localKey)

	s.mu.Lock()
	s.backupVault = candidate
	s.mu.Unlock()

	changes = append(changes, storage.Change{
		Path: s.path(), Contents: sealed,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(current)},
	})
	_, err = s.transactions.Commit(storage.Request{
		Operation: operation,
		Changes:   changes,
		Removals:  removals,
	})
	s.mu.Lock()
	s.backupVault = nil
	if err != nil {
		candidate.Destroy()
		s.vault.Destroy()
		s.vault = nil
		s.baseline = nil
		s.used = time.Time{}
		s.mu.Unlock()
		return err
	}

	if s.vault != nil && s.vault != candidate {
		s.vault.Destroy()
	}
	s.passwordless = passwordless
	s.vault = candidate
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.refusals = 0
	s.lastMigration = migration
	s.mu.Unlock()
	return nil
}

// Lock は、導出された鍵と未使用のトークンをすべて忘れる。
//
// ロック前に発行したトークンで資格情報を取得できないよう、トークンも削除する。
func (s *Service) Lock() {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.backupVault != nil && s.backupVault != s.vault {
		s.backupVault.Destroy()
	}
	s.vault.Destroy()
	s.vault = nil
	s.backupVault = nil
	s.baseline = nil
	s.lastMigration = Migration{}
}

// Reload は、ディスク上の保管庫を読み直す。
//
// 同期が中身を差し替えたあと、走っているこの実行は古い秘密を配り続けてはならない。
// マスターパスワードはもう持っていないので、いま手にしている鍵で開く。
func (s *Service) Reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		// 閉じているなら、次のロック解除がディスクから読む。何もしないのが正しい。
		return nil
	}
	sealed, err := s.workspace.FileSystem().ReadFile(s.path())
	if err != nil {
		return err
	}
	opened, err := OpenWith(sealed, vault.key)
	if err != nil {
		return err
	}
	vault.Destroy()
	s.vault = opened
	s.baseline = slices.Clone(sealed)
	return nil
}

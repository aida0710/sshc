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
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.idle
}

// SetIdleTimeout は、これ以降に使う自動ロック時間を変更する。
// 0は自動ロックなしであり、手動Lockとengine終了による破棄は変えない。
func (s *Service) SetIdleTimeout(timeout time.Duration) {
	if timeout < 0 {
		panic("secret: negative idle timeout")
	}
	s.mutex.Lock()
	s.idle = timeout
	// すでに新しい期限を超えていれば、次のrequestを待たずに破棄する。
	s.open()
	s.mutex.Unlock()
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
		s.closeLocked()
		return nil
	}
	return s.vault
}

// closeLocked は、導出した鍵と開いていた vault を忘れる。s.mutex を持って呼ぶ。
//
// 開いていた vault を閉じたときは afterLock へ知らせる。手動のロックでも、アイドル
// による自動ロックでも、ここを通る。
func (s *Service) closeLocked() {
	wasOpen := s.vault != nil
	s.vault.Destroy()
	s.vault = nil
	s.baseline = nil
	s.lastMigration = Migration{}
	if wasOpen && s.afterLock != nil {
		s.afterLock()
	}
}

// SetAfterUnlock は、Unlock が vault を開いた直後に呼ぶ関数を取り付ける。vault の
// 鍵が無いとできない移行（同期状態の digest を鍵付きにする）を、開いたときに一度
// 行うためにある。
func (s *Service) SetAfterUnlock(afterUnlock func()) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.afterUnlock = afterUnlock
}

// SetReportKeyBoundMigrationFailure は、ロックを解除したときの古い形の移行
// （migrateKeyBoundArtifacts）が失敗したことを知らせる先を取り付ける。
//
// 移行に失敗してもロックの解除は止めない。検証に通らない平文のスニペットのように何度
// やっても移せないものは、知らせないとマスターパスワードを変えようとするまで分からない。
func (s *Service) SetReportKeyBoundMigrationFailure(report func(error)) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.keyBoundMigrationFailureReport = report
}

// SetAfterLock は、vault が閉じた直後に呼ぶ関数を取り付ける。vault の中の設定から
// 組んだもの（同期先のアクセスキーとシークレット）を、閉じた vault の外に残さない
// ためにある。
//
// afterLock は s.mutex を持ったまま呼ぶ。この Service を呼び返してはならない。
func (s *Service) SetAfterLock(afterLock func()) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.afterLock = afterLock
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
	s.mutex.Lock()
	s.unattended++
	s.mutex.Unlock()
	defer func() {
		s.mutex.Lock()
		s.unattended--
		s.mutex.Unlock()
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
// 存在そのものにある。メニューバーを開くたびに呼ばれるので、読めば vault 全体
// （暗号文とはいえ、保存された値の全部）がそのたびにプロセスのメモリを通る。
func (s *Service) Exists() (bool, error) {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()
	return s.exists()
}

// exists は mutationMutex をすでに保持する operation が存在だけを読む。
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
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()

	exists, err := s.exists()
	if err != nil {
		return State{}, err
	}
	passwordless, err := s.hasLocalKey()
	if err != nil {
		return State{}, err
	}
	s.mutex.Lock()
	if !exists {
		// disk 上の vault を失ったあとも導出済み key だけを使い続けない。
		s.closeLocked()
	}
	unlocked := s.open() != nil
	migration := s.lastMigration
	s.mutex.Unlock()
	return State{Exists: exists, Unlocked: unlocked, LastMigration: migration, Passwordless: passwordless}, nil
}

// Unlocked は、このセッションでパスフレーズが与えられたかを報告する。
func (s *Service) Unlocked() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.open() != nil
}

// Initialise は、vault を持たないワークスペースのために vault を作る。
//
// すでに存在する場合は置き換えずに拒否する。うっかり再初期化すれば、保存済みの
// パスワードがすべて破壊されるし、鍵が失われた暗号化ファイルに復旧の道は
// ない。
func (s *Service) Initialise(passphrase string) error {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()
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
		Operation: operationInitialise,
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
	s.mutex.Lock()
	s.vault.Destroy()
	s.vault = vault
	s.passwordless = passphrase == ""
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.lastMigration = Migration{}
	s.mutex.Unlock()
	return nil
}

// Verify は、passphrase がこのワークスペースのマスターパスワードかを報告する。
//
// ファイルから判定し、ロックの状態を含めて何も変えない。したがってロック中の vault
// にも尋ねられる。CLI の change-password は、現在のパスワードを打ち込んだ直後に
// /cli/vault/verify からこれを呼び、誤りなら新しいパスワードを尋ねない。
// ChangeMasterPassword も、書き換える前にこれで現在のパスワードを確かめる。
//
// 誤りには Unlock と同じく refuse で待つ。/cli/vault/verify には handoff の
// シークレットを持つどのプロセスからでも届くので、ここで待たないと、Unlock の待ちを
// 避けてパスワードを次々に試す道になる。
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
			s.refuse()
			return false, nil
		}
		return false, err
	}
	vault.Destroy()
	s.mutex.Lock()
	// Unlock と同じく、通ったパスワードは誤りが積み上げた待ちを消す。
	s.refusals = 0
	s.mutex.Unlock()
	return true, nil
}

// MaxUnlockDelay は、拒否が待つ最長時間。
//
// vault ファイルはコピーしてオフラインで攻撃できるので、これは攻撃者とその中身の
// あいだに立つものではない、それは Argon2id である。これが止めるのは安価な場合だ。
// すなわち、動作中のアプリケーションに対して、判定できる限りの速さでパスワードを
// 試すローカルのプロセスである。
const MaxUnlockDelay = 4 * time.Second

// unlockDelayStepは、この実行で拒否が1回続くごとに増やす待ち。1回目の打ち間違いは
// ほとんど待たせず、16回続けて拒否したところでMaxUnlockDelayに達する。
const unlockDelayStep = 250 * time.Millisecond

// SetSleep は、拒否がどう待つかを取り付ける。バックオフを消費せずに観測するテスト
// のためのものである。
func (s *Service) SetSleep(sleep func(time.Duration)) { s.sleep = sleep }

// refuse は、この実行での連続した拒否が招いた分だけ待つ。
func (s *Service) refuse() {
	s.mutex.Lock()
	s.refusals++
	count := s.refusals
	s.mutex.Unlock()

	delay := min(time.Duration(count)*unlockDelayStep, MaxUnlockDelay)
	sleep := s.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	sleep(delay)
}

// Unlock は passphrase で vault を開き、開けたら afterUnlock へ知らせる。
//
// afterUnlock は変更のロックを放してから呼ぶ。知らされた側は、vault の鍵を使う
// 書き込み（同期状態の移行など）をこの Service を通して行ってよい。
func (s *Service) Unlock(passphrase string) error {
	s.mutationMutex.Lock()
	err := s.unlockHeld(passphrase)
	s.mutationMutex.Unlock()
	if err != nil {
		return err
	}
	s.notifyUnlocked()
	return nil
}

// notifyUnlocked は、SetAfterUnlock で取り付けた先へ、vault を開いたことを知らせる。
// 呼び手は mutationMutex を持っていてはならない。
func (s *Service) notifyUnlocked() {
	s.mutex.Lock()
	afterUnlock := s.afterUnlock
	s.mutex.Unlock()
	if afterUnlock != nil {
		afterUnlock()
	}
}

// unlockHeld は Unlock の本体である。呼び手が mutationMutex を持っている。
func (s *Service) unlockHeld(passphrase string) error {
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
	// 公開した vault は、この関数の外でアイドルのロックが閉じうる。下の移行に使う鍵は
	// 写しを持つ。
	key := vault.key.Clone()
	defer key.Destroy()
	s.mutex.Lock()
	s.passwordless = passwordless
	s.mutex.Unlock()
	if migration.Applied() {
		migrated, sealErr := vault.Seal()
		if sealErr != nil {
			vault.Destroy()
			return &MigrationError{From: migration.From, To: migration.To, Cause: sealErr}
		}
		if err := s.replaceVault(sealed, migrated, vault, nil, nil, "secret.migrate-vault", migration); err != nil {
			return err
		}
	} else {
		s.mutex.Lock()
		if s.vault != nil && s.vault != vault {
			s.vault.Destroy()
		}
		s.vault = vault
		s.baseline = slices.Clone(sealed)
		s.used = s.now()
		s.lastMigration = Migration{}
		// 通ったパスワードは、誤ったものが積み上げたものを消し去る。
		s.refusals = 0
		s.mutex.Unlock()
	}
	// 旧バージョンが残した保護文書と控えは、ロックを解除したこの機会に今の形へ移す。
	// 移せなくても（保留中の変更がある、など）ロックの解除は止めない。鍵を変える変更が
	// 封じ直す前にもう一度移す。
	if err := s.migrateKeyBoundArtifacts(keyBoundMigration{Key: key, Passphrase: passphrase}); err != nil {
		s.reportKeyBoundMigrationFailure(err)
	}
	return nil
}

// reportKeyBoundMigrationFailure は、ロックを解除したときの移行の失敗を、
// SetReportKeyBoundMigrationFailure で取り付けた先へ渡す。保留中の変更があって移せ
// なかったときは、それが片付いたあとの機会に移すので渡さない。
func (s *Service) reportKeyBoundMigrationFailure(err error) {
	if errors.Is(err, storage.ErrPendingTransaction) {
		return
	}
	s.mutex.Lock()
	report := s.keyBoundMigrationFailureReport
	s.mutex.Unlock()
	if report != nil {
		report(err)
	}
}

// RecoverCompatibleBackup は、現在のvaultが復号後のschema不一致だった場合に限り、
// 同じmaster passwordで開ける直近の現行schema世代へ戻す。世代backupはvault鍵で
// 二重に封じられているが、envelope headerがsaltを運ぶためpassphraseから直接開ける。
func (s *Service) RecoverCompatibleBackup(passphrase string) error {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()

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
		if migrateErr := s.migrateKeyBoundArtifacts(keyBoundMigration{Key: previous, Passphrase: passphrase}); migrateErr != nil {
			previous.Destroy()
			candidate.Destroy()
			return migrateErr
		}
		documents, reSealErr := s.reSealKeyBoundArtifacts(keyBoundReSeal{
			Vault: candidate, Previous: previous, Passphrase: passphrase,
			SkipBackup: true, IncludeSettings: true,
		})
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
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()

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
	if err := s.migrateKeyBoundArtifacts(keyBoundMigration{Key: previous, Passphrase: passphrase}); err != nil {
		previous.Destroy()
		candidate.Destroy()
		return err
	}
	documents, err := s.reSealKeyBoundArtifacts(keyBoundReSeal{
		Vault: candidate, Previous: previous, Passphrase: passphrase, SkipBackup: true,
	})
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

	s.mutex.Lock()
	s.backupVault = candidate
	s.mutex.Unlock()

	changes = append(changes, storage.Change{
		Path: s.path(), Contents: sealed,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(current)},
	})
	_, err = s.transactions.Commit(storage.Request{
		Operation: operation,
		Changes:   changes,
		Removals:  removals,
	})
	s.mutex.Lock()
	s.backupVault = nil
	if err != nil {
		candidate.Destroy()
		s.closeLocked()
		s.used = time.Time{}
		s.mutex.Unlock()
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
	s.mutex.Unlock()
	return nil
}

// Lock は、導出された鍵と、開いていた vault と backup の復号済みの内容を忘れる。
//
// パスワードなしの Vault でも鍵を破棄する。engine の終了処理が使う。利用者の操作に
// よるロックは LockManually を使う。
func (s *Service) Lock() {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()
	s.lockHeld()
}

// LockManually は、利用者の操作（画面のロックボタンや sshc vault lock）でロックする。
//
// パスワードなしの Vault は手動のロックを受け付けない。ロックしても、解錠に要る
// 鍵がこのマシンにあるので守るものが無いからである。そのときは解錠したままにし、
// 施錠中なら解錠する。判定と遷移を同じ mutationMutex の中で行い、あいだに別の変更を
// 挟ませない。
func (s *Service) LockManually() error {
	s.mutationMutex.Lock()
	defer s.mutationMutex.Unlock()
	passwordless, err := s.hasLocalKey()
	if err != nil {
		return err
	}
	if !passwordless {
		s.lockHeld()
		return nil
	}
	return s.unlockPasswordlessHeld()
}

// lockHeld は Lock の本体である。呼び手が mutationMutex を持っている。
func (s *Service) lockHeld() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.backupVault != nil && s.backupVault != s.vault {
		s.backupVault.Destroy()
	}
	s.backupVault = nil
	s.closeLocked()
}

// ReloadAfterRecovery は、履歴の画面などで片付けた保留記録が vault のファイルに
// 触れていれば、ディスク上の保管庫を読み直す。読み直さないと、ディスクは新しい
// vault、メモリは古い vault のままになり、消したはずのパスワードを配り続け、次の
// 書き込みは baseline の食い違いで失敗し続ける。
func (s *Service) ReloadAfterRecovery(paths []string) error {
	if !slices.Contains(paths, s.path()) {
		return nil
	}
	return s.Reload()
}

// Reload は、ディスク上の保管庫を読み直す。
//
// 同期が中身を差し替えたあと、走っているこの実行は古い秘密を配り続けてはならない。
// マスターパスワードはもう持っていないので、いま手にしている鍵で開く。
func (s *Service) Reload() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
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

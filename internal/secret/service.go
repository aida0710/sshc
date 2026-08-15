package secret

import (
	"crypto/subtle"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"sshc/internal/envelope"
	"sshc/internal/storage"
)

var (
	// ErrLocked は、このセッションでパスフレーズがまだ与えられていないことを報告する。
	ErrLocked = errors.New("the password vault is locked")
	// ErrAlreadyExists は、すでに vault を持つワークスペースに対して Initialise が
	// 呼ばれたことを報告する。上書きすれば、保存済みのパスワードがすべて破壊され、
	// 元に戻す手段はない。
	ErrAlreadyExists = errors.New("this workspace already has a password vault")
	// ErrNoVault は、まだ何も作られていないことを報告する。
	ErrNoVault = errors.New("this workspace has no password vault yet")
	// ErrUnknownToken は、発行されていない、すでに使われた、期限が切れた、あるいは
	// 別の alias に対して発行された askpass トークンを報告する。
	ErrUnknownToken = errors.New("that askpass token is not valid for this request")
	// ErrNoPassword は、その alias に何も保存されていないことを報告する。
	ErrNoPassword = errors.New("no password is stored for that host")
	// ErrNoPasswordMutation は、要求が vault の意味上の状態を変えないことを報告する。
	ErrNoPasswordMutation = errors.New("password mutation makes no change")
	// ErrCredentialAlreadyExists は、新規共有資格情報が既存名を上書きするのを防ぐ。
	ErrCredentialAlreadyExists = errors.New("a credential of that kind already has that name")
	// ErrUnknownPasswordMutation は接続作成が扱う三つのパスワード源以外を拒否する。
	ErrUnknownPasswordMutation = errors.New("that is not a password mutation kind")
)

// PasswordMutationKind は、接続作成が vault に行う変更を表す。専用パスワードと
// 名前付き資格情報は保存構造が異なるため、単なるオプションフィールドの組ではなく
// 判別可能な種類として運ぶ。
type PasswordMutationKind string

const (
	PasswordMutationDedicated PasswordMutationKind = "dedicated_password"
	PasswordMutationSaved     PasswordMutationKind = "saved_password"
	PasswordMutationNewShared PasswordMutationKind = "new_shared_password"
	PasswordMutationRemove    PasswordMutationKind = "remove"
)

// PasswordMutation は接続 alias に一つのパスワード源を割り当てる要求である。
// Password はこのパッケージの外へ返らず、commit callback に渡す storage.Change にも
// 封じられた bytes としてしか現れない。
type PasswordMutation struct {
	Kind       PasswordMutationKind
	Alias      string
	Credential string
	Password   string
}

// KeyPassphraseMutation replaces the unlock value owned by one private-key
// path. Unlike a named credential, it cannot be reused by another key.
type KeyPassphraseMutation struct {
	RelativePath string
	Passphrase   string
}

// ConnectionSecretsMutation groups every vault change made by one connection
// save so callers can commit one sealed replacement beside the SSH config.
type ConnectionSecretsMutation struct {
	Password      *PasswordMutation
	KeyPassphrase *KeyPassphraseMutation
}

// TokenTTL は、askpass トークンが使える時間。
//
// セッションのアクショントークンと同じ 2 分であり、理由も同じである。これは
// ユーザーがボタンを押してから OpenSSH がパスワードのプロンプトに到達するまでの
// 間隔であって、誰かが計画を立てられるようなウィンドウではない。
const TokenTTL = 2 * time.Minute

// IdleTimeout は、開いた vault が使われないまま生き続ける時間。
//
// プロセスの寿命のあいだずっと開いたままの vault は、ノートパソコンが鞄の中に
// あるあいだも開いている。しかもそれは、すべてのパスワードとすべての鍵の
// パスフレーズを保持している。8 時間は 1 日の勤務時間だ。朝に使った人は午後に
// 再度尋ねられず、夜に手を止めた人は尋ねられる。
const IdleTimeout = 8 * time.Hour

// Service は、プロセスの寿命のあいだ、開いた vault を所有する。
//
// 導出された鍵はこの構造体の中にだけあり、他のどこにもない。書き出されず、ログにも
// 出ず、返されることもない。外へ出るのはパスワードひとつだけであり、それも、この
// サービスが発行したトークンをひとつ持つ askpass リクエストひとつに対してである。
type Service struct {
	workspace    *storage.Workspace
	transactions *storage.Manager
	now          func() time.Time

	// sleep は拒否がどう待つかを表す。テストがバックオフを実際に消費せずに観測できる
	// よう注入する。
	sleep func(time.Duration)

	// mutationMu は vault の disk と memory の版をまたぐ変更を直列化する。storage
	// commit はバックアップを封じるために下の mu を再取得するので、commit 中に保持
	// するのはこちらだけである。
	mutationMu sync.Mutex
	mu         sync.Mutex
	vault      *Vault
	baseline   []byte
	// refusals は、連続して誤ったマスターパスワードの回数を数える。これが、拒否のたび
	// に前回より遅く答えさせている。
	refusals int
	// used は、秘密が最後に読み書きされた時刻。ステータスの読み取りは意図的に「使用」
	// に含めない。開いたブラウザタブは画面がマウントされるたびにそれを尋ねるので、
	// 忘れられたタブがひとつあるだけで、マシンの電源が入っているあいだじゅう vault が
	// 開いたままになってはならない。
	used time.Time
}

// State は status surface が一度に公開する vault の状態である。
type State struct {
	Exists   bool
	Unlocked bool
}

// NewService はロックされたサービスを返す。Unlock まで何も読めない。
func NewService(workspace *storage.Workspace, transactions *storage.Manager, now func() time.Time) *Service {
	return &Service{
		workspace:    workspace,
		transactions: transactions,
		now:          now,
	}
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
	if s.now().Sub(s.used) >= IdleTimeout {
		s.vault = nil
		s.baseline = nil
		return nil
	}
	return s.vault
}

// use は vault を返し、アイドルの時計をゼロに戻す。これは、秘密があるかどうかを
// 報告するのではなく、まさに秘密を読み書きしようとするときにメソッドが呼ぶもので
// ある。
func (s *Service) use() *Vault {
	vault := s.open()
	if vault != nil {
		s.used = s.now()
	}
	return vault
}

func (s *Service) path() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(WorkspacePath))
}

// Exists は vault ファイルが存在するかを報告する。これは、それがロック解除されて
// いるかという問いとは別のものである。
//
// **中身は読まない。** 有るか無いかを尋ねているだけであり、答えはファイルの
// 存在そのものにある。ここが `ReadFile` だったころは、メニューバーを開くたびに
// vault 全体（暗号文とはいえ、保存された答えの全部）が読み込まれてプロセスの
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

// State は disk 上の存在と memory 上の解錠状態を同じ mutation 境界で読む。
// status polling は秘密を使わないため、idle deadline は更新しない。
func (s *Service) State() (State, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	exists, err := s.exists()
	if err != nil {
		return State{}, err
	}
	s.mu.Lock()
	if !exists {
		// disk 上の vault を失ったあとも導出済み key だけを使い続けない。
		s.vault = nil
		s.baseline = nil
	}
	unlocked := s.open() != nil
	s.mu.Unlock()
	return State{Exists: exists, Unlocked: unlocked}, nil
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
	vault, err := Create(passphrase)
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.vault = vault
	s.used = s.now()
	s.mu.Unlock()
	if err := s.write(); err != nil {
		// disk に公開できなかった vault を memory だけで使える状態にしない。
		s.mu.Lock()
		s.vault = nil
		s.baseline = nil
		s.used = time.Time{}
		s.mu.Unlock()
		return err
	}
	return nil
}

// Verify は、passphrase がこのワークスペースのマスターパスワードかを報告する。
//
// ファイルから答え、何も変えない。したがって閉じた vault にも尋ねられるし、画面は、
// ユーザーが打ち込んだものをマスターパスワードとして使う前に、それがマスター
// パスワードかどうかを知ることができる。スナップショットを二つ目のパスワードでは
// なくマスターパスワードで封じられるのはこれのおかげだ。打ち間違いは、誰にも開け
// ないアーカイブではなく、ここでの拒否になる。
//
// コストは導出 1 回分で、ロック解除と同じである。しかもここに到達するのは、人が
// 求めた操作からだけだ。
func (s *Service) Verify(passphrase string) (bool, error) {
	sealed, err := s.workspace.FileSystem().ReadFile(s.path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, ErrNoVault
		}
		return false, err
	}
	if _, err := Open(sealed, passphrase); err != nil {
		if errors.Is(err, ErrWrongPassphrase) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// MaxUnlockDelay は、拒否が待つ最長時間。
//
// vault ファイルはコピーしてオフラインで攻撃できるので、これは攻撃者とその中身の
// あいだに立つものではない — それは Argon2id である。これが止めるのは安価な場合だ。
// すなわち、動作中のアプリケーションに対して、答えられる限りの速さでパスワードを
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
	sealed, err := s.workspace.FileSystem().ReadFile(s.path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNoVault
		}
		return err
	}
	vault, err := Open(sealed, passphrase)
	if err != nil {
		if errors.Is(err, ErrWrongPassphrase) {
			s.refuse()
		}
		return err
	}

	s.mu.Lock()
	s.vault = vault
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	// 通ったパスワードは、誤ったものが積み上げたものを消し去る。
	s.refusals = 0
	s.mu.Unlock()
	return nil
}

// Lock は、導出された鍵と未使用のトークンをすべて忘れる。
//
// トークンも一緒に消えるのは、ロックより長生きするトークンがあれば、ロック前に
// 始まった接続がロック後もパスワードを取得できてしまうからである。
func (s *Service) Lock() {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vault = nil
	s.baseline = nil
}

// Has は、alias にパスワードが保存されているかを報告する。ロック中はエラーでは
// なく false を答える。「見えない」と「存在しない」は外からは同じに見えるし、
// インターフェースは、どちらの状態にあるかを別途示している
// からである。
func (s *Service) Has(alias string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return false
	}
	_, ok := vault.SecretFor(KindPassword, alias)
	return ok
}

// Set は、alias ひとつ分のパスワードを保存し、vault を書き込む。
//
// 資格情報は alias を名前として取る。秘密に名前が付いたいま、「このホストのために
// とにかくパスワードを保存する」とはそういう意味である。複数のホストで共有するに
// は、代わりに既存の名前を割り当てる。
func (s *Service) Set(alias, password string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	if err := vault.SetDedicatedPassword(alias, password); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.write()
}

// Remove はパスワードを忘れ、vault を書き込む。
func (s *Service) Remove(alias string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	// 参照は消える。他に何かが指していれば資格情報は残り、何も指さなくなれば
	// 一緒に消える。
	vault.RemoveDedicatedPassword(alias)
	vault.Unassign(KindPassword, alias)
	_ = vault.Delete(KindPassword, alias)
	s.mu.Unlock()
	return s.write()
}

// Rename は、保存済みのパスワードを新しい alias へ引き継ぐ。ホストの名前変更が
// それを置き去りにすれば、二度と誰も尋ねない名前の下にパスワードが残る。
func (s *Service) Rename(from, to string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	if err := vault.Rename(KindPassword, from, to); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.write()
}

// Credentials は、両方の種別のすべての資格情報名と、その使用先を列挙する。
//
// 名前と使用先だけで、値は決して返さない。これは各画面が読むものであり、秘密を
// 読める画面があれば、それは侵害されたブラウザがそこから秘密を読める画面だという
// ことになる。
func (s *Service) Credentials() (map[Kind]map[string][]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return nil, ErrLocked
	}
	listed := map[Kind]map[string][]string{}
	for _, kind := range []Kind{KindPassword, KindKeyPassphrase} {
		listed[kind] = map[string][]string{}
		for _, name := range vault.Names(kind) {
			uses := vault.Uses(kind, name)
			if uses == nil {
				uses = []string{}
			}
			listed[kind][name] = uses
		}
	}
	return listed, nil
}

// SetCredential は、資格情報を作るか、その値を置き換える。
//
// 置き換えは、共有された秘密をローテーションする方法である。その名前を指している
// すべての subject が新しい値を読む。名前が存在する理由そのものだ。
func (s *Service) SetCredential(kind Kind, name, value string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	if err := vault.Set(kind, name, value); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.write()
}

// DeleteCredential は資格情報を忘れる。何かがそれを指しているあいだは拒否する。
func (s *Service) DeleteCredential(kind Kind, name string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	if err := vault.Delete(kind, name); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.write()
}

// AssignCredential は、subject を同じ種別の資格情報に向ける。種別が防護である。
// 他方の種別の名前が現れるマップは存在しない。
func (s *Service) AssignCredential(kind Kind, subject, name string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	if err := vault.Assign(kind, subject, name); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return s.write()
}

// UnassignCredential は subject の参照を忘れ、資格情報自体は残す。
func (s *Service) UnassignCredential(kind Kind, subject string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	vault.Unassign(kind, subject)
	s.mu.Unlock()
	return s.write()
}

// AssignedCredential は、subject が参照している名前を報告する。
func (s *Service) AssignedCredential(kind Kind, subject string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return "", false
	}
	return vault.Assigned(kind, subject)
}

// PasswordFor は、alias を、それに与えるべき値へ解決する。存在しないとき、および
// vault が閉じているときは "" を返す。重要な呼び出し側 — askpass の応答 — は、
// Redeem のエラーで両者を区別する。
func (s *Service) PasswordFor(alias string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return ""
	}
	value, _ := vault.SecretFor(KindPassword, alias)
	return value
}

// KeyPassphraseFor は、鍵のワークスペース相対パスを、保存済みのパスフレーズへ
// 解決する。鍵を二段階ではなく一度の操作でエージェントへ追加できるのはこれの
// おかげであり、鍵 vault が import するのではなく、そこへ注入される。
func (s *Service) KeyPassphraseFor(relativePath string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return "", false
	}
	return vault.SecretFor(KindKeyPassphrase, relativePath)
}

// HasKeyPassphrase reports whether an unlocked vault can resolve the key
// subject without returning its value across the service boundary.
func (s *Service) HasKeyPassphrase(relativePath string) bool {
	_, ok := s.KeyPassphraseFor(relativePath)
	return ok
}

// RelocateKeyPassphrases は鍵のパス変更に名前付きパスフレーズの割り当てを
// 追従させる。秘密の値には触れず、vault 内の subject 参照だけを一度に移す。
func (s *Service) RelocateKeyPassphrases(relocations map[string]string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	clone := vault.clone()
	changed, err := clone.RelocateSubjects(KindKeyPassphrase, relocations)
	baseline := slices.Clone(s.baseline)
	s.mu.Unlock()
	if err != nil || !changed {
		return err
	}
	if len(baseline) == 0 {
		return ErrNoVault
	}
	sealed, err := clone.Seal()
	if err != nil {
		return err
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "secret.key-passphrase-relocate",
		Changes: []storage.Change{{
			Path: s.path(), Contents: sealed,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(baseline)},
		}},
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.vault = clone
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.mu.Unlock()
	return nil
}

// SealBackup は世代バックアップをひとつ封じ、OpenBackup はそれを開く。
//
// これらはストレージ層に import されるのではなく、そこへ渡される。秘密がどこに
// あるかはこのパッケージの領分であり、トランザクションマネージャがそれを知らねば
// ならない道理はない。閉じた vault は何も封じず、何も開かない — アプリケーションは
// マスターパスワードの後ろにあるので、何かが書かれている最中にそれが起きることは
// ないし、万一起きたなら、ここで失敗するのが正しい答えである。もう一方の選択肢は、
// 秘密鍵の平文コピーだからだ。
func (s *Service) SealBackup(plaintext []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return nil, ErrLocked
	}
	return vault.SealBytes(plaintext)
}

func (s *Service) OpenBackup(sealed []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return nil, ErrLocked
	}
	return vault.OpenBytes(sealed)
}

// settingsPath は、vault の隣にある、封をされたオブジェクトストアの設定。
func (s *Service) settingsPath() string {
	return filepath.Join(s.workspace.Root(), filepath.FromSlash(SettingsPath))
}

// SyncSettings は、秘密も含めてオブジェクトストアの設定を返す。
//
// これを求めるのはクライアントを組み立てる呼び出し側だけである。画面には、秘密で
// ないフィールドから答える。これらを一度も与えられていないマシンはゼロ値を返し、
// エラーにはしない。「まだ設定されていない」は状態であって、失敗では
// ないからだ。
func (s *Service) SyncSettings() (SyncSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.use()
	if vault == nil {
		return SyncSettings{}, ErrLocked
	}
	sealed, err := s.workspace.FileSystem().ReadFile(s.settingsPath())
	if errors.Is(err, fs.ErrNotExist) {
		return SyncSettings{}, nil
	}
	if err != nil {
		return SyncSettings{}, err
	}
	return vault.OpenSettings(sealed)
}

// SetSyncSettings は、オブジェクトストアの設定を置き換える。
func (s *Service) SetSyncSettings(settings SyncSettings) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	sealed, err := vault.SealSettings(settings)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	current, readErr := s.workspace.FileSystem().ReadFile(s.settingsPath())
	precondition := storage.Precondition{}
	if readErr == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	}
	_, err = s.transactions.Commit(storage.Request{
		Operation: "sync.settings",
		Changes: []storage.Change{{
			Path: s.settingsPath(), Contents: sealed, Precondition: precondition,
		}},
	})
	return err
}

// write は vault を封じ、トランザクションマネージャを通してコミットする。これに
// より、書きかけの秘密ファイルは、このアプリケーションが到達しうる状態ではなくなる。
//
// 他のすべての書き込みと同じく、世代は保持される。バックアップ自体もこの vault の
// 鍵で封じられているので、vault の古い世代は、生きたファイルのコピーが明かす以上の
// ものを明かさない — そしてここでの事故は、他の手段では取り消せない数少ないものの
// ひとつである。
func (s *Service) write() error {
	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	sealed, err := vault.Seal()
	s.mu.Unlock()
	if err != nil {
		return err
	}

	if err := s.workspace.EnsureDirectory(s.workspace.StateDir()); err != nil {
		return err
	}
	current, readErr := s.workspace.FileSystem().ReadFile(s.path())
	precondition := storage.Precondition{}
	if readErr == nil {
		precondition = storage.Precondition{Exists: true, Digest: storage.Digest(current)}
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}

	_, err = s.transactions.Commit(storage.Request{
		Operation: "secret.vault",
		Changes: []storage.Change{{
			Path:         s.path(),
			Contents:     sealed,
			Precondition: precondition,
		}},
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.baseline = slices.Clone(sealed)
	// A token is a capability for the value observed at issuance. Any vault
	// write may change an assignment or shared value, so invalidate all of
	// them instead of trying to infer which references moved.
	s.mu.Unlock()
	return nil
}

// WithPasswordMutation prepares a password-vault replacement and lets the
// application commit it beside the SSH configuration change. The live vault
// remains unchanged until that callback succeeds. mutationMu stays held so a
// second writer cannot overtake the transaction; mu is deliberately released
// while storage runs because storage seals generational backups through this
// same service.
func (s *Service) WithPasswordMutation(
	mutation PasswordMutation,
	commit func(storage.Change) (storage.Result, error),
) (storage.Result, error) {
	return s.WithConnectionSecretsMutation(ConnectionSecretsMutation{Password: &mutation}, commit)
}

// WithConnectionSecretsMutation prepares one encrypted vault replacement for
// all secret changes belonging to a connection save. Disk and live memory are
// published only after the caller's combined storage transaction succeeds.
func (s *Service) WithConnectionSecretsMutation(
	mutation ConnectionSecretsMutation,
	commit func(storage.Change) (storage.Result, error),
) (storage.Result, error) {
	return s.WithConnectionSecretsTransaction(mutation, func(change *storage.Change) (storage.Result, error) {
		if change == nil {
			if mutation.Password != nil && mutation.Password.Kind == PasswordMutationRemove {
				return storage.Result{}, ErrNoPassword
			}
			return storage.Result{}, ErrNoPasswordMutation
		}
		return commit(*change)
	})
}

// WithConnectionSecretsTransaction keeps vault writers serialized across a
// connection commit even when the requested cleanup is a semantic no-op. A nil
// change lets the caller commit config without needlessly sealing the vault.
func (s *Service) WithConnectionSecretsTransaction(
	mutation ConnectionSecretsMutation,
	commit func(*storage.Change) (storage.Result, error),
) (storage.Result, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()

	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		exists, err := s.exists()
		if err != nil {
			return storage.Result{}, err
		}
		if !exists {
			if mutation.Password != nil && mutation.Password.Kind == PasswordMutationRemove && mutation.KeyPassphrase == nil {
				return commit(nil)
			}
			return storage.Result{}, ErrNoVault
		}
		return storage.Result{}, ErrLocked
	}
	clone := vault.clone()
	changed := false
	if mutation.Password != nil {
		passwordChanged, err := applyPasswordMutation(vault, clone, *mutation.Password)
		if errors.Is(err, ErrNoPassword) && mutation.Password.Kind == PasswordMutationRemove {
			err = nil
			passwordChanged = false
		}
		if err != nil {
			s.mu.Unlock()
			return storage.Result{}, err
		}
		changed = changed || passwordChanged
	}
	if mutation.KeyPassphrase != nil {
		keyMutation := mutation.KeyPassphrase
		current, hasDedicated := vault.dedicatedKeyPassphrases[keyMutation.RelativePath]
		keyChanged := !hasDedicated || len(current) != len(keyMutation.Passphrase) ||
			subtle.ConstantTimeCompare([]byte(current), []byte(keyMutation.Passphrase)) != 1
		if keyChanged {
			if err := clone.SetDedicatedKeyPassphrase(keyMutation.RelativePath, keyMutation.Passphrase); err != nil {
				s.mu.Unlock()
				return storage.Result{}, err
			}
			changed = true
		}
	}
	if !changed {
		s.mu.Unlock()
		return commit(nil)
	}
	sealed, err := clone.Seal()
	baseline := slices.Clone(s.baseline)
	s.mu.Unlock()
	if err != nil {
		return storage.Result{}, err
	}
	if len(baseline) == 0 {
		return storage.Result{}, ErrNoVault
	}

	change := storage.Change{
		Path: s.path(), Contents: sealed,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(baseline)},
	}
	result, err := commit(&change)
	if err != nil {
		return storage.Result{}, err
	}
	s.mu.Lock()
	s.vault = clone
	s.baseline = slices.Clone(sealed)
	s.used = s.now()
	s.mu.Unlock()
	return result, nil
}

func applyPasswordMutation(vault, clone *Vault, mutation PasswordMutation) (bool, error) {
	switch mutation.Kind {
	case PasswordMutationDedicated:
		if current, ok := vault.dedicatedPasswords[mutation.Alias]; ok &&
			len(current) == len(mutation.Password) &&
			subtle.ConstantTimeCompare([]byte(current), []byte(mutation.Password)) == 1 {
			return false, nil
		}
		if err := clone.SetDedicatedPassword(mutation.Alias, mutation.Password); err != nil {
			return false, err
		}
		return true, nil
	case PasswordMutationSaved:
		if current, ok := vault.Assigned(KindPassword, mutation.Alias); ok && current == mutation.Credential {
			return false, nil
		}
		if err := clone.Assign(KindPassword, mutation.Alias, mutation.Credential); err != nil {
			return false, err
		}
		return true, nil
	case PasswordMutationNewShared:
		if _, exists := vault.Secret(KindPassword, mutation.Credential); exists {
			return false, ErrCredentialAlreadyExists
		}
		if err := clone.Set(KindPassword, mutation.Credential, mutation.Password); err != nil {
			return false, err
		}
		if err := clone.Assign(KindPassword, mutation.Alias, mutation.Credential); err != nil {
			return false, err
		}
		return true, nil
	case PasswordMutationRemove:
		if _, ok := clone.SecretFor(KindPassword, mutation.Alias); !ok {
			return false, ErrNoPassword
		}
		clone.RemoveDedicatedPassword(mutation.Alias)
		clone.Unassign(KindPassword, mutation.Alias)
		return true, nil
	default:
		return false, ErrUnknownPasswordMutation
	}
}

// DedicatedKeyPassphrases returns only key paths with a non-reusable value.
// Locked vaults reveal no subjects.
func (s *Service) DedicatedKeyPassphrases() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return nil
	}
	return vault.DedicatedKeyPassphraseSubjects()
}

// Aliases は、パスワードが保存されているホストを返す。ロック中は何も返さない。
func (s *Service) Aliases() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	vault := s.open()
	if vault == nil {
		return nil
	}
	return vault.Subjects(KindPassword)
}

// ChangeMasterPassword は、鍵を導出し直し、その鍵が保持していたすべてを封じ直す。
//
// vault も、封をされたオブジェクトストアの設定も、すべての世代バックアップも、
// マスターパスワードから導出された鍵で封じられている。vault だけを置き換える変更は、
// 残りを、もう誰も使わないパスワードで開ける状態のまま残す。それは失うのと同じ
// ことだ。バックアップは、そこから復元するために存在するのであり、誰にも開けない
// バックアップはバックアップではない。
//
// トランザクションはひとつ。置き換えるものの世代コピーは保持しない。そしてそこが、
// SkipBackup がいまも正しい唯一の場所である。古い鍵で封じられた古い vault のコピー
// は、これが終わった瞬間に開けなくなるからだ。すべてはジャーナルにステージされる
// ので、中断されても完了させられる。できないのは巻き戻しであり、Rollback はそれを
// できるふりをせずに述べる。
//
// リモートのスナップショットを封じ直すのはこの関数の仕事ではない。それはオブジェクト
// ストアのものであり、このパッケージはそれを import しない。push は呼び出し側が行う。
func (s *Service) ChangeMasterPassword(current, next string) error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if ok, err := s.Verify(current); err != nil {
		return err
	} else if !ok {
		return ErrWrongPassphrase
	}

	s.mu.Lock()
	vault := s.use()
	if vault == nil {
		s.mu.Unlock()
		return ErrLocked
	}
	previous, err := vault.Rekey(next)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	// ここから先、vault は新しい鍵を保持する。したがって、これが封じるものは
	// 新しい鍵で封じられ、これ以前に封じられたものは古い鍵で開かれる。
	changes, buildErr := s.reSealed(vault, previous)
	if buildErr != nil {
		// 古い鍵を戻す。何も書かれていないので、メモリ上の vault はディスク上の
		// vault と一致し続けなければならない。
		vault.key = previous
		s.mu.Unlock()
		return buildErr
	}
	sealed, sealErr := vault.Seal()
	s.mu.Unlock()
	if sealErr != nil {
		return sealErr
	}

	previousVault, readErr := s.workspace.FileSystem().ReadFile(s.path())
	if readErr != nil {
		return readErr
	}
	changes = append(changes, storage.Change{
		Path: s.path(), Contents: sealed, SkipBackup: true,
		Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(previousVault)},
	})
	if _, err := s.transactions.Commit(storage.Request{
		Operation: "secret.rekey",
		Changes:   changes,
	}); err != nil {
		s.mu.Lock()
		vault.key = previous
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.baseline = slices.Clone(sealed)
	s.mu.Unlock()
	return nil
}

// reSealed は、古い鍵が封じたすべてのファイルを読み、新しい鍵で封じ直す。
//
// バックアップはマネージャ経由ではなく直接読む。マネージャは、サービスがいま
// 保持している鍵で開くからだ — そしてこれが走る時点では、それは新しい鍵で
// ある。
func (s *Service) reSealed(vault *Vault, previous envelope.Key) ([]storage.Change, error) {
	changes := make([]storage.Change, 0, 8)

	settings, err := s.workspace.FileSystem().ReadFile(s.settingsPath())
	switch {
	case err == nil:
		plaintext, openErr := previous.Open(settings)
		if openErr != nil {
			return nil, openErr
		}
		resealed, sealErr := vault.SealBytes(plaintext)
		if sealErr != nil {
			return nil, sealErr
		}
		changes = append(changes, storage.Change{
			Path: s.settingsPath(), Contents: resealed, SkipBackup: true,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(settings)},
		})
	case errors.Is(err, fs.ErrNotExist):
	default:
		return nil, err
	}

	backups := filepath.Join(s.workspace.StateDir(), storage.BackupDirectoryName)
	walkErr := filepath.WalkDir(backups, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		body, readErr := s.workspace.FileSystem().ReadFile(path)
		if readErr != nil {
			return readErr
		}
		plaintext, openErr := previous.Open(body)
		if openErr != nil {
			// バックアップがそもそも封じられるようになる前に書かれたものは、この
			// 関数が変換すべきものではない。そのひとつのために変更全体を拒むのは、
			// そのまま残すより悪い。
			return nil
		}
		resealed, sealErr := vault.SealBytes(plaintext)
		if sealErr != nil {
			return sealErr
		}
		changes = append(changes, storage.Change{
			Path: path, Contents: resealed, SkipBackup: true,
			Precondition: storage.Precondition{Exists: true, Digest: storage.Digest(body)},
		})
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.ErrNotExist) {
		return nil, walkErr
	}
	return changes, nil
}

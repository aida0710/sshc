package keys

import (
	"context"
	"errors"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"sshc/internal/config"
	"sshc/internal/platform"
	"sshc/internal/storage"
	"sshc/internal/validate"
)

var (
	ErrUnknownKey                  = errors.New("no key with that identifier is in the inventory")
	ErrInvalidFileName             = errors.New("file name is not a safe single path segment")
	ErrInvalidComment              = errors.New("comment contains characters this application will not put in a command line")
	ErrConflictingPassphraseChoice = errors.New("a passphrase was supplied together with the unencrypted flag")
	ErrUnknownGroup                = errors.New("no declared group of that name")
	ErrKeyNotEncrypted             = errors.New("this private key is not encrypted")
	// ErrKeyChanged は、確かめたあとで鍵のファイルが置き換わった、編集された、または
	// 消えたことを報告する。パスフレーズの検証と保存のあいだ、走査と agent への登録の
	// あいだに起きる。
	ErrKeyChanged = errors.New("the key file changed after it was checked")
)

// KeysDirectoryName は、グループごとにサブディレクトリをひとつ持つディレクトリ。
// 設定エンジンが所有する connections ディレクトリを映したものである。import すると
// 依存の向きが逆になるので、import ではなく名前を重複させてある。
const KeysDirectoryName = "keys"

// fileNamePattern は、安全なパスセグメントをひとつだけ受け付ける。先頭のドット、
// スラッシュ、'..' セグメントはこのパターンでは不可能なので、
// Workspace.ResolveForWrite が見る前から、~/.ssh の外のファイルは指定できない。
var fileNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// hardwareCommentPattern は、シェルの引用を必要としない文字だけを受け付ける。
// ハードウェア鍵のコメントは、コピー可能な ssh-keygen のコマンドラインの中に表示
// され、ユーザーはその行を自分で実行するからである。
var hardwareCommentPattern = regexp.MustCompile(`^[A-Za-z0-9@._+=:,/-]{0,127}$`)

// commentPattern は、このアプリケーション自身が埋め込むコメントのルール。
//
// ソフトウェア鍵はプロセス内で生成され、そのコメントは ssh.MarshalPrivateKey へ
// 渡るので、シェルには届かず引用も要らない。ハードウェア用のルールを当てると
// 空白を拒否することになる。空白は普通のコメントに最も入りやすい文字であり、
// `ssh-keygen -C "work laptop"` がそこに入れる文字である。ここで拒否するのは、
// ファイルや、それが書かれる行を壊すもの、すなわち改行、復帰、ヌルで
// ある。
var commentPattern = regexp.MustCompile(`^[^\x00\r\n]{0,127}$`)

// safeArgumentPattern は、このアプリケーションが表示するコマンドラインの各要素に
// 最後に適用される検査。
//
// 区切り文字はこの OS のものである。Windows の絶対パスは `\` を含み、それを
// 落とすと `-f` に渡す鍵のパスが常に弾かれ、ハードウェア鍵のコマンドラインは
// あの OS で一度も組み立てられない。逆に Unix で `\` を許せば、表示した行が
// 貼り付け先の sh でエスケープとして読まれる。だから許すのは、その OS が実際に
// パスの区切りに使う文字だけである。
var safeArgumentPattern = regexp.MustCompile(`^[A-Za-z0-9@%_+=:,./` + regexp.QuoteMeta(string(filepath.Separator)) + `-]+$`)

// ValidateFileName は、安全な単一パスセグメントでないものをすべて拒否する。
func ValidateFileName(name string) error {
	if !fileNamePattern.MatchString(name) {
		return ErrInvalidFileName
	}
	if strings.HasSuffix(name, ".pub") || name == StateDirectoryName {
		return ErrInvalidFileName
	}
	if validate.Reserved(name) {
		return ErrInvalidFileName
	}
	return nil
}

// ValidateComment は、鍵ファイルや、それが書かれる行を壊すコメントを拒否する。
// これは、このアプリケーションが自ら生成する鍵のためのルールである。
func ValidateComment(comment string) error {
	if !commentPattern.MatchString(comment) {
		return ErrInvalidComment
	}
	return nil
}

// ValidateHardwareComment は、ハードウェア鍵のために表示する ssh-keygen の
// コマンドライン内で、引用しなければならなくなるコメントを拒否する。
func ValidateHardwareComment(comment string) error {
	if !hardwareCommentPattern.MatchString(comment) {
		return ErrInvalidComment
	}
	return nil
}

// Service は鍵 vault のユースケース層。HTTP も UI の関心事も持たず、読み取りは
// ストレージのファイルシステムのインターフェースを通してのみ、書き込みはジャーナル付きの
// トランザクションマネージャを通してのみ行う。
type Service struct {
	workspace    *storage.Workspace
	transactions *storage.Manager
	resolver     config.Resolver
	catalogue    CatalogueReader
	agent        platform.KeyAgent
	now          func() time.Time
	random       io.Reader
	// validateGroup を注入するのは、グループとは何かが設定エンジンの領分だからである
	//（Include 行が宣言したときにグループは存在する）そしてこのパッケージは、それを
	// 尋ねるためにそのエンジンを import してはならない。
	validateGroup func(string) error
	// storedPassphrase は、ある鍵のために保持されているパスフレーズがあり、vault が
	// 開いていれば、それを返す。注入する理由は validateGroup と同じである。秘密が
	// どこにあるかは secret パッケージの領分であり、このパッケージはそれを尋ねるために
	// import してはならない。nil なら、どの鍵にもパスフレーズが保存されていないものと
	// して扱う。
	storedPassphrase func(relativePath string) (string, bool)
}

type ServiceOptions struct {
	Workspace        *storage.Workspace
	Transactions     *storage.Manager
	Resolver         config.Resolver
	StoredPassphrase func(relativePath string) (string, bool)
	Catalogue        CatalogueReader
	Agent            platform.KeyAgent
	Now              func() time.Time
	Random           io.Reader
	ValidateGroup    func(string) error
}

func NewService(options ServiceOptions) *Service {
	return &Service{
		workspace:        options.Workspace,
		transactions:     options.Transactions,
		resolver:         options.Resolver,
		catalogue:        options.Catalogue,
		agent:            options.Agent,
		now:              options.Now,
		random:           options.Random,
		validateGroup:    options.ValidateGroup,
		storedPassphrase: options.StoredPassphrase,
	}
}

// SetStoredPassphrase は、構築後にこの参照関数を取り付ける。尋ねる相手の vault より
// 先にこのサービスを組み立てる配線のためである。
func (service *Service) SetStoredPassphrase(lookup func(relativePath string) (string, bool)) {
	service.storedPassphrase = lookup
}

// PassphraseVerification は復号成功を inventory item と検査済みの鍵内容へ関連付ける。
// 鍵素材や秘密は含まない。
type PassphraseVerification struct {
	KeyID        string
	RelativePath string
	Digest       string
}

// VerifyPassphrase は現在の inventory ID だけを解決し、暗号化秘密鍵に対して、入力された
// パスフレーズでその内容を復号できることを検証する。呼び出し側の入力と一時ファイルの
// バッファは消去する。
func (service *Service) VerifyPassphrase(keyID string, passphrase []byte) (PassphraseVerification, error) {
	defer clear(passphrase)
	_, item, err := service.privateKey(keyID)
	if err != nil {
		return PassphraseVerification{}, err
	}
	if !item.Encrypted {
		return PassphraseVerification{}, ErrKeyNotEncrypted
	}
	contents, err := service.workspace.FileSystem().ReadFile(service.absolutePath(item.RelativePath))
	if err != nil {
		return PassphraseVerification{}, err
	}
	defer clear(contents)
	digest := storage.Digest(contents)
	if _, err := DecodePrivateKey(contents, passphrase); err != nil {
		return PassphraseVerification{}, err
	}
	return PassphraseVerification{
		KeyID: keyID, RelativePath: item.RelativePath, Digest: digest,
	}, nil
}

// RevalidatePassphrase はトランザクションの commit 前に対象鍵が置換、編集、削除された場合、
// 古い検証結果を拒否する。
func (service *Service) RevalidatePassphrase(verification PassphraseVerification) error {
	contents, err := service.workspace.FileSystem().ReadFile(service.absolutePath(verification.RelativePath))
	if err != nil {
		return ErrKeyChanged
	}
	defer clear(contents)
	if storage.Digest(contents) != verification.Digest {
		return ErrKeyChanged
	}
	return nil
}

// entryPath は、Include グラフの起点となるユーザー設定ファイル。
func (service *Service) entryPath() string {
	return service.absolutePath("config")
}

func (service *Service) absolutePath(relativePath string) string {
	return filepath.Join(service.workspace.Root(), relativePath)
}

// Inventory はワークスペースを分類し、各ファイルを指定する Host を付与する。
func (service *Service) Inventory() (*Inventory, error) {
	inventory, err := NewScanner(service.workspace).Scan()
	if err != nil {
		return nil, err
	}
	graph, err := service.resolver.Resolve(service.entryPath())
	if err != nil {
		return inventory, nil
	}
	inventory.AttachReferences(BuildReferenceIndex(graph, service.workspace))
	return inventory, nil
}

// privateKey は、いま走査した inventory から keyID の秘密鍵を引く。見つからないか、
// 秘密鍵でなければ ErrUnknownKey を返す。秘密鍵を対象にする操作は、どれもこれで対象を
// 決めるので、対象にする鍵の条件を変えるときはここだけを直す。inventory も返すのは、
// 公開鍵の片割れやコメントを同じ走査の結果から探す呼び出し側のため。
func (service *Service) privateKey(keyID string) (*Inventory, *Item, error) {
	inventory, err := service.Inventory()
	if err != nil {
		return nil, nil, err
	}
	item, ok := inventory.Find(keyID)
	if !ok || item.Kind != KindPrivateKey {
		return nil, nil, ErrUnknownKey
	}
	return inventory, item, nil
}

// Algorithms は、インストールされている OpenSSH が対応する variant を報告する。
func (service *Service) Algorithms(ctx context.Context) Catalogue {
	return service.catalogue.Read(ctx)
}

// HardwareCommand は、ハードウェアの方式に対する ssh-keygen の引数リストを返す。
//
// このコマンドはグループ配下のフルパスを指定するので、ユーザーが手で実行しても、
// このアプリケーションが置いたであろう場所にちょうど鍵が置かれる。
func (service *Service) HardwareCommand(algorithm Algorithm, fileName, group, comment string) ([]string, error) {
	directory, err := service.groupDirectory(group)
	if err != nil {
		return nil, err
	}
	return HardwareCommand(algorithm, fileName, comment, service.absolutePath(directory))
}

// groupDirectory はグループを検証し、その鍵が置かれるワークスペース相対の
// ディレクトリを返す。グループが空の場合は ~/.ssh のルートで、グループなしの鍵は
// そこに属する。
func (service *Service) groupDirectory(group string) (string, error) {
	if group == "" {
		return ".", nil
	}
	if service.validateGroup == nil {
		return "", ErrUnknownGroup
	}
	if err := service.validateGroup(group); err != nil {
		return "", err
	}
	return path.Join(KeysDirectoryName, group), nil
}

// GenerateRequest は、プロセス内での鍵生成ひとつ分。
//
// パスフレーズを空にするには Unencrypted を明示的に設定しなければならない。うっかり
// 空欄になったフィールドから、保護されない鍵が暗黙に生まれることは決してない。
type GenerateRequest struct {
	Algorithm Algorithm
	Bits      int
	FileName  string
	// Group は、そのグループの鍵ディレクトリにペアを置く。FileName の一部ではなく、
	// 別個に検証される独立したフィールドである。呼び出し側から渡されたグループを名前へ
	// 連結すると、鍵が "config" に書かれるのを止めている単一セグメントのルールを
	// 破ってしまうからだ。
	Group       string
	Comment     string
	Passphrase  []byte
	Unencrypted bool
}

type GenerateResult struct {
	ID                 string
	RelativePath       string
	PublicRelativePath string
	Fingerprint        string
	KeyType            string
	Bits               int
	Encrypted          bool
	TransactionID      string
}

// Generate は、このプロセス内でソフトウェアの鍵ペアを作り、両方のファイルを
// ひとつのジャーナル付きトランザクションでコミットする。パスフレーズが argv、環境、
// 別プロセスに届くことは決してなく、Generate が返る前に上書きされる。
func (service *Service) Generate(request GenerateRequest) (GenerateResult, error) {
	defer clear(request.Passphrase)

	if err := ValidateFileName(request.FileName); err != nil {
		return GenerateResult{}, err
	}
	if err := ValidateComment(request.Comment); err != nil {
		return GenerateResult{}, err
	}
	directory, err := service.groupDirectory(request.Group)
	if err != nil {
		return GenerateResult{}, err
	}
	if len(request.Passphrase) == 0 && !request.Unencrypted {
		return GenerateResult{}, ErrPassphraseRequired
	}
	if len(request.Passphrase) > 0 && request.Unencrypted {
		return GenerateResult{}, ErrConflictingPassphraseChoice
	}

	privateKey, err := GeneratePrivateKey(request.Algorithm, request.Bits, service.random)
	if err != nil {
		return GenerateResult{}, err
	}
	privateContents, err := EncodePrivateKey(privateKey, request.Comment, request.Passphrase)
	if err != nil {
		return GenerateResult{}, err
	}
	defer clear(privateContents)
	publicContents, err := EncodePublicKey(privateKey, request.Comment)
	if err != nil {
		return GenerateResult{}, err
	}
	info, err := InspectPublicKey(publicContents)
	if err != nil {
		return GenerateResult{}, err
	}

	privateName := path.Join(directory, request.FileName)
	publicName := privateName + ".pub"
	privatePath, publicPath := service.absolutePath(privateName), service.absolutePath(publicName)
	result, err := service.transactions.Commit(storage.Request{
		Operation:   "key.generate",
		Directories: storage.ParentDirectoryCreates(service.workspace.Root(), []string{privatePath, publicPath}),
		Changes: []storage.Change{
			{Path: privatePath, Contents: privateContents},
			{Path: publicPath, Contents: publicContents},
		},
	})
	if err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{
		ID:                 ItemID(privateName),
		RelativePath:       privateName,
		PublicRelativePath: publicName,
		Fingerprint:        info.Fingerprint,
		KeyType:            info.KeyType,
		Bits:               info.Bits,
		Encrypted:          len(request.Passphrase) > 0,
		TransactionID:      result.ID,
	}, nil
}

// PassphraseChange は、秘密鍵ひとつを再暗号化する。
type PassphraseChange struct {
	KeyID       string
	Current     []byte
	New         []byte
	Unencrypted bool
}

type PassphraseResult struct {
	ID            string
	RelativePath  string
	Encrypted     bool
	Notes         []string
	TransactionID string
}

// ChangePassphrase は、現在のパスフレーズで鍵を復号し、新しいパスフレーズで暗号化
// して書き戻す。読み取ったファイルのダイジェストで守られた、ひとつのジャーナル付き
// トランザクションで行う。
//
// 置き換える前の秘密鍵は、ほかの変更と同じく世代バックアップに残す。控えはVaultの鍵で
// 封じるので、~/.ssh/sshc/backups/に平文の鍵は残らない。ただしパスワードなしのVaultでは、
// 封じる鍵を導く値（~/.ssh/sshc/local-vault-key）も同じマシンにある。新しい鍵を設置する
// renameは原子的なので、中断されても古い鍵か新しい鍵のどちらかが残り、中断された変更は
// 完了させることも巻き戻すこともできる。
//
// x/crypto のパーサは OpenSSH の秘密鍵の中に保存されたコメントを公開しないので、
// コメントは、フィンガープリントが一致する公開鍵ファイルから取る。そうしたファイル
// が存在しなければ、新しい鍵はコメントを持たず、結果は NoteCommentNotPreserved で
// その旨を伝える。エンジンがコメントをでっちあげることは決してない。
func (service *Service) ChangePassphrase(change PassphraseChange) (PassphraseResult, error) {
	defer clear(change.Current)
	defer clear(change.New)

	if len(change.New) == 0 && !change.Unencrypted {
		return PassphraseResult{}, ErrPassphraseRequired
	}
	if len(change.New) > 0 && change.Unencrypted {
		return PassphraseResult{}, ErrConflictingPassphraseChoice
	}

	inventory, item, err := service.privateKey(change.KeyID)
	if err != nil {
		return PassphraseResult{}, err
	}

	absolute := service.absolutePath(item.RelativePath)
	contents, err := service.workspace.FileSystem().ReadFile(absolute)
	if err != nil {
		return PassphraseResult{}, err
	}
	defer clear(contents)
	precondition := storage.Precondition{Exists: true, Digest: storage.Digest(contents)}

	privateKey, err := DecodePrivateKey(contents, change.Current)
	if err != nil {
		return PassphraseResult{}, err
	}
	comment, notes := commentForKey(inventory, item)
	encoded, err := EncodePrivateKey(privateKey, comment, change.New)
	if err != nil {
		return PassphraseResult{}, err
	}
	defer clear(encoded)

	result, err := service.transactions.Commit(storage.Request{
		Operation: "key.passphrase",
		Changes: []storage.Change{{
			Path:         absolute,
			Contents:     encoded,
			Precondition: precondition,
		}},
	})
	if err != nil {
		return PassphraseResult{}, err
	}
	return PassphraseResult{
		ID:            item.ID,
		RelativePath:  item.RelativePath,
		Encrypted:     len(change.New) > 0,
		Notes:         notes,
		TransactionID: result.ID,
	}, nil
}

// RevealResult は、確認済みの秘密鍵表示に対する結果。
type RevealResult struct {
	ID            string
	RelativePath  string
	Contents      []byte
	Encrypted     bool
	Fingerprint   string
	TransactionID string
}

// Reveal は、秘密鍵ひとつのバイト列を返す。
//
// 監査記録はバイト列が返される前に書かれるので、記録できなかった表示は起こらない。
// 記録はファイルと時刻を指定し、鍵素材を含むことは決してない。Reveal に他の
// 呼び出し側が存在しないのは意図的である。通常の詳細 API が秘密鍵のバイト列を
// 返すことはない。
func (service *Service) Reveal(keyID string) (RevealResult, error) {
	_, item, err := service.privateKey(keyID)
	if err != nil {
		return RevealResult{}, err
	}

	absolute := service.absolutePath(item.RelativePath)
	contents, err := service.workspace.FileSystem().ReadFile(absolute)
	if err != nil {
		return RevealResult{}, err
	}
	result, err := service.transactions.Note("key.reveal", []string{absolute})
	if err != nil {
		clear(contents)
		return RevealResult{}, err
	}
	return RevealResult{
		ID:            item.ID,
		RelativePath:  item.RelativePath,
		Contents:      contents,
		Encrypted:     item.Encrypted,
		Fingerprint:   item.Fingerprint,
		TransactionID: result.ID,
	}, nil
}

// PublicKeyResult は、公開鍵ファイルまたは証明書ファイルひとつのテキスト。
type PublicKeyResult struct {
	ID           string
	RelativePath string
	Contents     string
	Fingerprint  string
	Comment      string
}

// PublicKey は、公開鍵または証明書ひとつのテキストを返す。
//
// これは意図的に Reveal ではない。公開鍵は秘密ではないので、確認も、監査の記録も、
// Cache-Control の小細工もない。それらは秘密鍵の素材が開示されるからこそ存在する
// のであって、ここでは何ひとつ開示されない。そしてそれを真に保っているのが kind の
// 検査である。スキャナが公開鍵または証明書と分類したものだけを受け付けるので、
// 秘密鍵の識別子がこの関数に届いても、それは読まれるのではなく拒否される。分類は
// .pub という接尾辞ではなく内容と権限によって行われるので、id_rsa.pub と誤って
// 名付けられた秘密鍵についても、同じく拒否
// される。
func (service *Service) PublicKey(keyID string) (PublicKeyResult, error) {
	inventory, err := service.Inventory()
	if err != nil {
		return PublicKeyResult{}, err
	}
	item, ok := inventory.Find(keyID)
	if !ok || (item.Kind != KindPublicKey && item.Kind != KindCertificate) {
		return PublicKeyResult{}, ErrUnknownKey
	}
	contents, err := service.workspace.FileSystem().ReadFile(service.absolutePath(item.RelativePath))
	if err != nil {
		return PublicKeyResult{}, err
	}
	return PublicKeyResult{
		ID:           item.ID,
		RelativePath: item.RelativePath,
		Contents:     string(contents),
		Fingerprint:  item.Fingerprint,
		Comment:      item.Comment,
	}, nil
}

// commentForKey は、同じフィンガープリントを持つ公開鍵ファイルから秘密鍵の
// コメントを復元する。
func commentForKey(inventory *Inventory, item *Item) (string, []string) {
	if item.Fingerprint != "" {
		for _, candidate := range inventory.Items {
			if candidate.Kind == KindPublicKey && candidate.Fingerprint == item.Fingerprint && candidate.Comment != "" {
				return candidate.Comment, nil
			}
		}
	}
	return "", []string{NoteCommentNotPreserved}
}

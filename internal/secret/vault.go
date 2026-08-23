// Package secret は、OpenSSH 接続で使用する資格情報を暗号化して保存する。
// 保存先はワークスペース内の ~/.ssh/sshc/secrets で、暗号鍵は保存しない
// マスターパスワードから導出する。
package secret

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"

	"sshc/internal/envelope"
	"sshc/internal/validate"
)

// WorkspacePath は、暗号化されたファイルの置き場所。ワークスペースルートからの
// 相対である。エディタに開いてくれと誘うような拡張子を持たず、読めそうに見える
// 名前も持たない。
const WorkspacePath = "sshc/secrets"

// SettingsPath は、マスターパスワードで暗号化したオブジェクトストア設定の保存先。
// 同期先への資格情報がスナップショットに含まれないよう、vault とは別に保存する。
const SettingsPath = "sshc/sync-settings"

// SchemaVersion は、暗号化の内側にある平文文書のバージョン。ヘッダーは envelope
// 用に自前のバージョンを運ぶ。
const SchemaVersion = 3

// envelope のエラーは再エクスポートしてある。vault を扱う呼び出し側が、どの
// パッケージがそれを暗号化したかを知らずに済むようにするためだ。
var (
	ErrWrongPassphrase    = envelope.ErrWrongPassphrase
	ErrNotAVault          = envelope.ErrNotAnEnvelope
	ErrUnsupportedVersion = envelope.ErrUnsupportedVersion
	ErrCostRefused        = envelope.ErrCostRefused
	ErrWeakPassphrase     = envelope.ErrWeakPassphrase
)

var (
	// ErrUnsafeName は、安全な alias ではない alias を拒否する。
	ErrUnsafeName = errors.New("that is not a safe host alias")
	// ErrEmptySecret は空のパスワードを拒否する。プロンプト上では、誤ったものと
	// 区別がつかないからだ。
	ErrEmptySecret = errors.New("the password is empty")
	// ErrOldVault は、秘密に名前が付く前の文書を報告する。世界に多くともひとつしか
	// 存在せず、移行のコードは移行される対象より大きくなるので、拒否したうえで画面が
	// 最初からやり直すことを提案する。
	ErrOldVault = errors.New("this vault predates named credentials and cannot be read")
	// ErrUnknownKind は、どちらでもない名前空間を拒否する。
	ErrUnknownKind = errors.New("that is not a credential kind")
	// ErrUnknownCredential は、その名前空間に存在しない名前への参照を拒否する。
	// これは、ホストが鍵のパスフレーズを参照するのを止めている仕組みでも
	// ある。
	ErrUnknownCredential = errors.New("no credential of that kind has that name")
	// ErrCredentialInUse は、まだ何かが指している秘密の削除を拒む。
	ErrCredentialInUse = errors.New("something still uses this credential")
)

// MinPassphraseLength は、これが受け付ける最短の vault パスフレーズ長。
const MinPassphraseLength = envelope.MinPassphraseLength

// Kind は、資格情報の名前空間を表す。
//
// ホストは KindPassword のみを、鍵は KindKeyPassphrase のみを参照できる。名前空間が
// ひとつなら、ホストのパスワード選択画面が鍵のパスフレーズを提示できてしまい、それを
// 選べばそのパスフレーズがログインパスワードとしてリモートホストへ送られる。二つに
// 分ければ、それは起こりにくいどころか、表現すること自体が不可能になる。
//
// アカウントパスワードと秘密鍵パスフレーズを別の名前空間に分け、誤送信を防ぐ。
type Kind string

const (
	KindPassword      Kind = "password"
	KindKeyPassphrase Kind = "key_passphrase"
)

// ValidKind は、値が名前空間を表しているかを報告する。この集合が決まる唯一の場所
// なので、ルートとフォームがそれについて食い違うことはありえない。
func ValidKind(kind Kind) bool {
	return kind == KindPassword || kind == KindKeyPassphrase
}

// SyncSettings は、オブジェクトストアが必要とするもの。
//
// vault と同じマスターパスワードで暗号化し、同期対象外の専用ファイルに保存する。
type SyncSettings struct {
	Endpoint string `json:"endpoint,omitempty"`
	Bucket   string `json:"bucket,omitempty"`
	// Path は、すべてのオブジェクトが置かれる接頭辞。空ならバケットのルート。
	Path            string `json:"path,omitempty"`
	Region          string `json:"region,omitempty"`
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
	Direction       string `json:"direction,omitempty"`
	// Auto は、この設置で自動同期を入れてあるか。
	//
	// 他の同期の設定と同じ場所に住む。巡回に必要なものは、鍵も資格情報も
	// この入切も、すべて保管庫が開いてから読める。閉じている間は何も読めず、
	// 何も起きない。それがこの機能の唯一の条件である。
	Auto bool `json:"auto,omitempty"`
	// Key は、リモートのスナップショットを暗号化する値である。
	//
	// マスターパスワードではない。それを使っていたので、マスターパスワードが
	// 端末をまたいだ共有秘密になっていた。1 台で変えれば他の全端末が締め出され、
	// 打つユーザーが居ないところでは復号できなかった。ここに置くことで、
	// マスターパスワードは端末ごとのローカルな秘密に戻る。
	Key string `json:"key,omitempty"`
}

// document は平文であり、この形でこのパッケージの外へ出ることは決してない。
//
// 資格情報のマップが二つ、そして種別ごとに参照のマップがひとつ。ホストは alias で、
// 鍵はワークスペース相対のパスでキー付けされる。名前の付いた秘密は、いくつの
// subject が指していても一度だけ保存される。この形により、
// 20 台のマシンが共有するパスワードを、一か所でローテーションできる。
type document struct {
	SchemaVersion           int               `json:"schemaVersion"`
	Passwords               map[string]string `json:"passwords"`
	DedicatedPasswords      map[string]string `json:"dedicatedPasswords,omitempty"`
	KeyPassphrases          map[string]string `json:"keyPassphrases"`
	DedicatedKeyPassphrases map[string]string `json:"dedicatedKeyPassphrases,omitempty"`
	Hosts                   map[string]string `json:"hosts"`
	Keys                    map[string]string `json:"keys"`
}

// Vault は、開かれた secrets ファイル。
//
// パスフレーズではなく導出された鍵を保持するので、再度尋ねることなく変更を暗号化
// 直せる。そしてパスフレーズは、Open が返ったあとどこにも保持されて
// いない。
type Vault struct {
	key                     envelope.Key
	secrets                 map[Kind]map[string]string
	subjects                map[Kind]map[string]string
	dedicatedPasswords      map[string]string
	dedicatedKeyPassphrases map[string]string
}

func newMaps() (map[Kind]map[string]string, map[Kind]map[string]string) {
	return map[Kind]map[string]string{KindPassword: {}, KindKeyPassphrase: {}},
		map[Kind]map[string]string{KindPassword: {}, KindKeyPassphrase: {}}
}

// Create は、passphrase で暗号化された空の vault を返す。
func Create(passphrase string) (*Vault, error) {
	key, err := envelope.Derive(passphrase)
	if err != nil {
		return nil, err
	}
	secrets, subjects := newMaps()
	return &Vault{
		key: key, secrets: secrets, subjects: subjects,
		dedicatedPasswords:      map[string]string{},
		dedicatedKeyPassphrases: map[string]string{},
	}, nil
}

// Open は、passphrase で sealed を復号する。
func Open(sealed []byte, passphrase string) (*Vault, error) {
	plaintext, key, err := envelope.Open(sealed, passphrase)
	if err != nil {
		return nil, err
	}
	return openDocument(plaintext, key)
}

// OpenWith は、すでに導出してある鍵で vault を開く。
// 同期後の再読込ではマスターパスワードを保持していないため、この関数を使う。
func OpenWith(sealed []byte, key envelope.Key) (*Vault, error) {
	plaintext, err := key.Open(sealed)
	if err != nil {
		return nil, err
	}
	return openDocument(plaintext, key)
}

func openDocument(plaintext []byte, key envelope.Key) (*Vault, error) {
	var parsed document
	if err := json.Unmarshal(plaintext, &parsed); err != nil {
		return nil, ErrWrongPassphrase
	}
	if parsed.SchemaVersion > SchemaVersion {
		return nil, ErrUnsupportedVersion
	}
	// バージョン 1 の文書は、alias ごとにパスワードを持ち、名前をまったく持たな
	// かった。世界に多くともひとつしか存在せず、そのための移行は移行する対象より
	// 大きくなるので、暗黙に作り変えるのではなく、画面が「もう一度設定してください」
	// に変えられるエラーで拒否する。
	if parsed.SchemaVersion < 2 {
		return nil, ErrOldVault
	}
	secrets, subjects := newMaps()
	for kind, stored := range map[Kind]map[string]string{
		KindPassword:      parsed.Passwords,
		KindKeyPassphrase: parsed.KeyPassphrases,
	} {
		for name, value := range stored {
			secrets[kind][name] = value
		}
	}
	for kind, stored := range map[Kind]map[string]string{
		KindPassword:      parsed.Hosts,
		KindKeyPassphrase: parsed.Keys,
	} {
		for subject, name := range stored {
			subjects[kind][subject] = name
		}
	}
	dedicatedPasswords := maps.Clone(parsed.DedicatedPasswords)
	if dedicatedPasswords == nil {
		dedicatedPasswords = map[string]string{}
	}
	dedicatedKeyPassphrases := maps.Clone(parsed.DedicatedKeyPassphrases)
	if dedicatedKeyPassphrases == nil {
		dedicatedKeyPassphrases = map[string]string{}
	}
	return &Vault{
		key: key, secrets: secrets, subjects: subjects,
		dedicatedPasswords:      dedicatedPasswords,
		dedicatedKeyPassphrases: dedicatedKeyPassphrases,
	}, nil
}

// SealSettings は、オブジェクトストアの設定を vault 自身の鍵で暗号化する。隣に置く
// ファイルのためである。同じマスターパスワードで、違うファイル。こちらは移動
// しない。
func (v *Vault) SealSettings(settings SyncSettings) ([]byte, error) {
	plaintext, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	return v.key.Seal(plaintext)
}

// OpenSettings は、SealSettings が書いたファイルを復号する。
func (v *Vault) OpenSettings(sealed []byte) (SyncSettings, error) {
	plaintext, err := v.key.Open(sealed)
	if err != nil {
		return SyncSettings{}, err
	}
	var settings SyncSettings
	if err := json.Unmarshal(plaintext, &settings); err != nil {
		return SyncSettings{}, ErrWrongPassphrase
	}
	return settings, nil
}

// Seal は、書き込みのために vault を暗号化する。
// Rekey は passphrase から新しい鍵を導出し、それを採用する。
//
// 中身には手を触れない。変わるのは、それを開くものの方だ。古い鍵が暗号化したものは
// すべて、同じ流れの中で呼び出し側が暗号化し直さなければならない。これが新しい vault
// ではなく vault のメソッドである理由はそこにある。呼び出し側は、二つの鍵を同時に
// 必要とするからだ。
func (v *Vault) Rekey(passphrase string) (envelope.Key, error) {
	key, err := envelope.Derive(passphrase)
	if err != nil {
		return envelope.Key{}, err
	}
	previous := v.key
	v.key = key
	return previous, nil
}

// SealBytes は、任意のバイト列をこの vault の鍵で暗号化する。
//
// これが、世代バックアップのディレクトリを、以前のファイル内容の山、バックアップ
// をまったく拒んでいた書き込みについては、以前の秘密鍵そのもの、から、暗号文の
// 山へと変える。
func (v *Vault) SealBytes(plaintext []byte) ([]byte, error) {
	return v.key.Seal(plaintext)
}

// OpenBytes はその逆で、巻き戻しや復元のためにある。
func (v *Vault) OpenBytes(sealed []byte) ([]byte, error) {
	return v.key.Open(sealed)
}

// Empty は、この vault が資格情報も参照も保持していないことを報告する。
// 初回 pull では空のローカル vault を競合する編集として扱わない。
func (v *Vault) Empty() bool {
	for _, secrets := range v.secrets {
		if len(secrets) != 0 {
			return false
		}
	}
	for _, subjects := range v.subjects {
		if len(subjects) != 0 {
			return false
		}
	}
	return len(v.dedicatedPasswords) == 0 && len(v.dedicatedKeyPassphrases) == 0
}

// Document は、同期用に復号済みの vault 文書を返す。
// 呼び出し側は、この文書を同期鍵で暗号化したアーカイブにだけ格納する。
func (v *Vault) Document() ([]byte, error) {
	return json.Marshal(document{
		SchemaVersion:           SchemaVersion,
		Passwords:               v.secrets[KindPassword],
		DedicatedPasswords:      v.dedicatedPasswords,
		KeyPassphrases:          v.secrets[KindKeyPassphrase],
		DedicatedKeyPassphrases: v.dedicatedKeyPassphrases,
		Hosts:                   v.subjects[KindPassword],
		Keys:                    v.subjects[KindKeyPassphrase],
	})
}

func (v *Vault) Seal() ([]byte, error) {
	plaintext, err := v.Document()
	if err != nil {
		return nil, err
	}
	return v.key.Seal(plaintext)
}

// Names は、ある種別の資格情報名をソートして返す。名前そのものは秘密ではない。
// 秘密なのは、それが表す値の方である。
func (v *Vault) Names(kind Kind) []string {
	return slices.Sorted(maps.Keys(v.secrets[kind]))
}

// Secret は、名前が表す値を返す。
func (v *Vault) Secret(kind Kind, name string) (string, bool) {
	value, ok := v.secrets[kind][name]
	return value, ok
}

// SetDedicatedPassword stores a password whose owner is exactly one host.
// It is structurally separate from named credentials, so it cannot appear in a
// reusable-credential list or be assigned to another host.
func (v *Vault) SetDedicatedPassword(alias, value string) error {
	if err := validate.Alias(alias); err != nil {
		return ErrUnsafeName
	}
	if value == "" {
		return ErrEmptySecret
	}
	delete(v.subjects[KindPassword], alias)
	v.dedicatedPasswords[alias] = value
	return nil
}

// RemoveDedicatedPassword forgets a connection-owned password. A missing
// alias is already the requested state and is therefore not an error.
func (v *Vault) RemoveDedicatedPassword(alias string) {
	delete(v.dedicatedPasswords, alias)
}

// SetDedicatedKeyPassphrase stores an unlock value owned by exactly one private
// key. It is deliberately separate from named credentials so replacing it
// cannot rotate the passphrase used by any other key.
func (v *Vault) SetDedicatedKeyPassphrase(relativePath, value string) error {
	if relativePath == "" || strings.ContainsRune(relativePath, '\x00') {
		return ErrUnsafeName
	}
	if value == "" {
		return ErrEmptySecret
	}
	delete(v.subjects[KindKeyPassphrase], relativePath)
	v.dedicatedKeyPassphrases[relativePath] = value
	return nil
}

// RemoveKeyPassphrase forgets either representation attached to one key. It
// never removes the named credential itself because other keys may still use it.
func (v *Vault) RemoveKeyPassphrase(relativePath string) {
	delete(v.subjects[KindKeyPassphrase], relativePath)
	delete(v.dedicatedKeyPassphrases, relativePath)
}

// DedicatedKeyPassphraseSubjects lists only the key-owned entries, never their
// plaintext values.
func (v *Vault) DedicatedKeyPassphraseSubjects() []string {
	return slices.Sorted(maps.Keys(v.dedicatedKeyPassphrases))
}

// clone returns a fully independent plaintext document with the same derived
// key. Mutations made to it cannot become visible through the live vault until
// its owner explicitly publishes the clone after a successful disk commit.
func (v *Vault) clone() *Vault {
	secrets, subjects := newMaps()
	for kind := range secrets {
		secrets[kind] = maps.Clone(v.secrets[kind])
		subjects[kind] = maps.Clone(v.subjects[kind])
	}
	return &Vault{
		key: v.key, secrets: secrets, subjects: subjects,
		dedicatedPasswords:      maps.Clone(v.dedicatedPasswords),
		dedicatedKeyPassphrases: maps.Clone(v.dedicatedKeyPassphrases),
	}
}

// Set は、名前の下に資格情報を保存する。新規作成か、値の置き換えである。
func (v *Vault) Set(kind Kind, name, value string) error {
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	if !validCredentialName(name) {
		return ErrUnsafeName
	}
	if value == "" {
		return ErrEmptySecret
	}
	v.secrets[kind][name] = value
	return nil
}

// Delete は資格情報を忘れる。まだ何かが指しているあいだは拒否する。
//
// 秘密に名前を付ける意味は、多くの subject がひとつのエントリを共有することにある。
// だからその足元でエントリを取り除けば、あとになって、別のどこかで、すべての
// subject が一度に壊れることになる。
func (v *Vault) Delete(kind Kind, name string) error {
	if len(v.Uses(kind, name)) > 0 {
		return ErrCredentialInUse
	}
	delete(v.secrets[kind], name)
	return nil
}

// Uses は、資格情報を参照している subject をソートして列挙する。
func (v *Vault) Uses(kind Kind, name string) []string {
	var uses []string
	for subject, referenced := range v.subjects[kind] {
		if referenced == name {
			uses = append(uses, subject)
		}
	}
	slices.Sort(uses)
	return uses
}

// Assign は、subject を同じ種別の資格情報へ向ける。
//
// 種別こそが防護のすべてである。ホストは alias を名前とし、アカウントのパスワード
// だけを参照できる。鍵はワークスペース相対のパスを名前とし、鍵のパスフレーズだけを
// 参照できる。両者をまたぐ参照は、実行時の検査ではなく、
// 他方の種別の名前が現れるマップが、そもそも存在しないのである。
func (v *Vault) Assign(kind Kind, subject, name string) error {
	if !ValidKind(kind) {
		return ErrUnknownKind
	}
	if kind == KindPassword {
		if err := validate.Alias(subject); err != nil {
			return ErrUnsafeName
		}
	} else if subject == "" || strings.ContainsAny(subject, "\x00") {
		return ErrUnsafeName
	}
	if _, ok := v.secrets[kind][name]; !ok {
		return ErrUnknownCredential
	}
	if kind == KindPassword {
		delete(v.dedicatedPasswords, subject)
	} else {
		delete(v.dedicatedKeyPassphrases, subject)
	}
	v.subjects[kind][subject] = name
	return nil
}

// Unassign は subject の参照を忘れる。subject がなくてもエラーではない。
func (v *Vault) Unassign(kind Kind, subject string) {
	delete(v.subjects[kind], subject)
	if kind == KindKeyPassphrase {
		delete(v.dedicatedKeyPassphrases, subject)
	}
}

// Assigned は、subject が参照している資格情報を返す。
func (v *Vault) Assigned(kind Kind, subject string) (string, bool) {
	name, ok := v.subjects[kind][subject]
	return name, ok
}

// Subjects は、ある資格情報を参照している、ある種別のすべての subject を返す。
func (v *Vault) Subjects(kind Kind) []string {
	subjects := maps.Clone(v.subjects[kind])
	if kind == KindPassword {
		for alias := range v.dedicatedPasswords {
			subjects[alias] = ""
		}
	} else if kind == KindKeyPassphrase {
		for relativePath := range v.dedicatedKeyPassphrases {
			subjects[relativePath] = ""
		}
	}
	return slices.Sorted(maps.Keys(subjects))
}

// SecretFor は、subject を、それに与えるべき値へ解決する。
func (v *Vault) SecretFor(kind Kind, subject string) (string, bool) {
	if kind == KindPassword {
		if value, ok := v.dedicatedPasswords[subject]; ok {
			return value, true
		}
	} else if kind == KindKeyPassphrase {
		if value, ok := v.dedicatedKeyPassphrases[subject]; ok {
			return value, true
		}
	}
	name, ok := v.subjects[kind][subject]
	if !ok {
		return "", false
	}
	return v.Secret(kind, name)
}

// Rename は、subject の参照を新しい名前へ引き継ぐ。ホストの名前変更はこれを
// しなければならず、さもなければ参照は、誰も尋ねない名前の下に暗黙に孤児に
// なる。
func (v *Vault) Rename(kind Kind, from, to string) error {
	if kind == KindPassword {
		if value, ok := v.dedicatedPasswords[from]; ok {
			if err := validate.Alias(to); err != nil {
				return ErrUnsafeName
			}
			delete(v.dedicatedPasswords, from)
			delete(v.subjects[kind], to)
			v.dedicatedPasswords[to] = value
			return nil
		}
	}
	if kind == KindKeyPassphrase {
		if value, ok := v.dedicatedKeyPassphrases[from]; ok {
			if to == "" || strings.ContainsRune(to, '\x00') {
				return ErrUnsafeName
			}
			delete(v.dedicatedKeyPassphrases, from)
			delete(v.subjects[kind], to)
			v.dedicatedKeyPassphrases[to] = value
			return nil
		}
	}
	name, ok := v.subjects[kind][from]
	if !ok {
		return nil
	}
	if kind == KindPassword {
		if err := validate.Alias(to); err != nil {
			return ErrUnsafeName
		}
	}
	delete(v.subjects[kind], from)
	v.subjects[kind][to] = name
	return nil
}

// RelocateSubjects は複数の subject 名を一つのスナップショットとして移す。
//
// グループ名の変更では、親と子の鍵が同時に移動する。1 件ずつ Rename すると、
// ある移動先が別の移動元でもある場合に先の値を上書きし得るため、変更前の
// map から値を集め、すべての移動元を消してから移動先へ置く。
func (v *Vault) RelocateSubjects(kind Kind, relocations map[string]string) (bool, error) {
	if !ValidKind(kind) {
		return false, ErrUnknownKind
	}
	if len(relocations) == 0 {
		return false, nil
	}
	moved := make(map[string]string)
	movedDedicated := make(map[string]string)
	for from, to := range relocations {
		if from == to {
			continue
		}
		if kind == KindPassword {
			if err := validate.Alias(to); err != nil {
				return false, ErrUnsafeName
			}
		} else if to == "" || strings.ContainsRune(to, '\x00') {
			return false, ErrUnsafeName
		}
		if name, ok := v.subjects[kind][from]; ok {
			moved[to] = name
		}
		if kind == KindKeyPassphrase {
			if value, ok := v.dedicatedKeyPassphrases[from]; ok {
				movedDedicated[to] = value
				delete(moved, to)
			}
		}
	}
	if len(moved) == 0 && len(movedDedicated) == 0 {
		return false, nil
	}
	for from := range relocations {
		delete(v.subjects[kind], from)
		if kind == KindKeyPassphrase {
			delete(v.dedicatedKeyPassphrases, from)
		}
	}
	for to, name := range moved {
		if kind == KindKeyPassphrase {
			delete(v.dedicatedKeyPassphrases, to)
		}
		v.subjects[kind][to] = name
	}
	for to, value := range movedDedicated {
		delete(v.subjects[kind], to)
		v.dedicatedKeyPassphrases[to] = value
	}
	return true, nil
}

// validCredentialName は、ユーザーが打ち込み、画面が表示できる名前を受け付ける。これは
// alias ではない。資格情報は、それが何のためのものかにちなんで名付けられ、それは
// ホスト名ではなく「オフィスの VM 群」かもしれないからだ。
func validCredentialName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	return !strings.ContainsAny(name, "\x00\r\n")
}

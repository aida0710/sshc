package secret

import (
	"encoding/json"
	"errors"
	"maps"

	"sshc/internal/envelope"
	"sshc/internal/strictjson"
)

// SchemaVersion は、暗号化の内側にある平文文書のバージョン。ヘッダーは envelope
// 用に自前のバージョンを運ぶ。
const SchemaVersion = 6

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
	// ErrOlderSchema は、復号には成功したものの、平文documentが現行schemaより古い
	// ことを報告する。envelope形式の不一致と分けることで、画面が「新しすぎる」と
	// 誤案内せず、安全な復旧操作だけを提示できる。
	ErrOlderSchema = errors.New("the vault schema is older than this application supports")
	// ErrNewerSchema は、復号済みdocumentがこのbuildより新しいschemaを要求する。
	ErrNewerSchema = errors.New("the vault schema is newer than this application supports")
)

// SchemaVersionError は、復号後に判明したvault documentのバージョンの違いである。schema番号は
// 秘密ではなく、診断へ載せても資格情報やpassphraseを明かさない。
type SchemaVersionError struct {
	Found     int
	Supported int
}

func (e *SchemaVersionError) Error() string {
	return "vault schema version is not supported"
}

func (e *SchemaVersionError) Is(target error) bool {
	if target == ErrUnsupportedVersion {
		return true
	}
	if e.Found < e.Supported {
		return target == ErrOlderSchema
	}
	return target == ErrNewerSchema
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
	PasswordBindings        map[string]string `json:"passwordBindings,omitempty"`
	Keys                    map[string]string `json:"keys"`
	TOTPs                   map[string]string `json:"totps"`
	TOTPHosts               map[string]string `json:"totpHosts"`
	TOTPBindings            map[string]string `json:"totpBindings,omitempty"`
	// VPNs は、VPN プロファイルの秘密である。名前はプロファイル名で、subject を
	// 持たない。host にも鍵にも割り当てないからである。
	VPNs map[string]string `json:"vpns"`
}

// Create は、passphrase で暗号化された空の vault を返す。
func Create(passphrase string) (*Vault, error) {
	key, err := envelope.DeriveWithMinimum(passphrase, MinPassphraseLength)
	if err != nil {
		return nil, err
	}
	secrets, subjects := newMaps()
	return &Vault{
		key: key, secrets: secrets, subjects: subjects,
		dedicatedPasswords:      map[string]string{},
		passwordBindings:        map[string]string{},
		totpBindings:            map[string]string{},
		dedicatedKeyPassphrases: map[string]string{},
	}, nil
}

// Open は、passphrase で sealed を復号する。
func Open(sealed []byte, passphrase string) (*Vault, error) {
	vault, _, err := openSealedWithMigrations(sealed, passphrase, registeredDocumentMigrations)
	return vault, err
}

func openSealedWithMigrations(
	sealed []byte,
	passphrase string,
	migrations migrationRegistry,
) (*Vault, Migration, error) {
	plaintext, key, err := envelope.Open(sealed, passphrase)
	if err != nil {
		return nil, Migration{}, err
	}
	return openDocumentWithMigrations(plaintext, key, migrations)
}

// OpenWith は、すでに導出してある鍵で vault を開く。
// 同期後の再読込ではマスターパスワードを保持していないため、この関数を使う。
func OpenWith(sealed []byte, key envelope.Key) (*Vault, error) {
	plaintext, err := key.Open(sealed)
	if err != nil {
		return nil, err
	}
	return openDocument(plaintext, key.Clone())
}

func openDocument(plaintext []byte, key envelope.Key) (*Vault, error) {
	vault, _, err := openDocumentWithMigrations(plaintext, key, registeredDocumentMigrations)
	return vault, err
}

func openDocumentWithMigrations(
	plaintext []byte,
	key envelope.Key,
	migrations migrationRegistry,
) (*Vault, Migration, error) {
	// key is transferred into this function. Keep it only when a Vault is
	// successfully returned; malformed or unsupported documents must not leave
	// their derived key material waiting for the garbage collector.
	published := false
	defer func() {
		if !published {
			key.Destroy()
		}
	}()
	plaintext, migration, err := migrateDocument(plaintext, migrations)
	if err != nil {
		return nil, Migration{}, err
	}
	var parsed document
	if err := strictjson.Decode(plaintext, &parsed); err != nil {
		if migration.Applied() {
			return nil, Migration{}, &MigrationError{From: migration.From, To: migration.To, Cause: err}
		}
		return nil, Migration{}, ErrWrongPassphrase
	}
	if parsed.SchemaVersion != SchemaVersion {
		return nil, Migration{}, &SchemaVersionError{Found: parsed.SchemaVersion, Supported: SchemaVersion}
	}
	secrets, subjects := newMaps()
	for kind, stored := range map[Kind]map[string]string{
		KindPassword:      parsed.Passwords,
		KindKeyPassphrase: parsed.KeyPassphrases,
		KindTOTP:          parsed.TOTPs,
		KindVPN:           parsed.VPNs,
	} {
		for name, value := range stored {
			secrets[kind][name] = value
		}
	}
	for kind, stored := range map[Kind]map[string]string{
		KindPassword:      parsed.Hosts,
		KindKeyPassphrase: parsed.Keys,
		KindTOTP:          parsed.TOTPHosts,
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
	opened := &Vault{
		key: key, secrets: secrets, subjects: subjects,
		dedicatedPasswords:      dedicatedPasswords,
		passwordBindings:        maps.Clone(parsed.PasswordBindings),
		totpBindings:            maps.Clone(parsed.TOTPBindings),
		dedicatedKeyPassphrases: dedicatedKeyPassphrases,
	}
	published = true
	return opened, migration, nil
}

// Rekey は passphrase から新しい鍵を導出し、それを採用する。
//
// 中身には手を触れない。変わるのは、それを開くものの方だ。古い鍵が暗号化したものは
// すべて、同じ流れの中で呼び出し側が暗号化し直さなければならない。これが新しい vault
// ではなく vault のメソッドである理由はそこにある。呼び出し側は、二つの鍵を同時に
// 必要とするからだ。
func (v *Vault) Rekey(passphrase string) (envelope.Key, error) {
	key, err := envelope.DeriveWithMinimum(passphrase, MinPassphraseLength)
	if err != nil {
		return envelope.Key{}, err
	}
	previous := v.key
	v.key = key
	return previous, nil
}

// SealBytes は、任意のバイト列をこの vault の鍵で暗号化する。
//
// 世代バックアップはこれで封じる。置き換えられるファイルの前の内容（暗号化していない
// 秘密鍵を含む）が、平文のまま控えに残らないようにするためである。
func (v *Vault) SealBytes(plaintext []byte) ([]byte, error) {
	return v.key.Seal(plaintext)
}

// OpenBytes はその逆で、巻き戻しや復元のためにある。
func (v *Vault) OpenBytes(sealed []byte) ([]byte, error) {
	return v.key.Open(sealed)
}

// Document は、同期用に復号済みの vault 文書を返す。
// 呼び出し側は、この文書を同期鍵で暗号化したアーカイブにだけ格納する。
func (v *Vault) Document() ([]byte, error) {
	if v.passwordBindings == nil {
		v.passwordBindings = map[string]string{}
	}
	if v.totpBindings == nil {
		v.totpBindings = map[string]string{}
	}
	return json.Marshal(document{
		SchemaVersion:           SchemaVersion,
		Passwords:               v.secrets[KindPassword],
		DedicatedPasswords:      v.dedicatedPasswords,
		KeyPassphrases:          v.secrets[KindKeyPassphrase],
		DedicatedKeyPassphrases: v.dedicatedKeyPassphrases,
		Hosts:                   v.subjects[KindPassword],
		PasswordBindings:        v.passwordBindings,
		Keys:                    v.subjects[KindKeyPassphrase],
		TOTPs:                   v.secrets[KindTOTP],
		TOTPHosts:               v.subjects[KindTOTP],
		TOTPBindings:            v.totpBindings,
		VPNs:                    v.secrets[KindVPN],
	})
}

// Seal は、書き込みのために vault を暗号化する。
func (v *Vault) Seal() ([]byte, error) {
	plaintext, err := v.Document()
	if err != nil {
		return nil, err
	}
	return v.key.Seal(plaintext)
}

// Destroy best-effort clears the independently owned derived key. Secret
// values are Go strings and cannot be reliably overwritten in place; they are
// released only when the Vault itself is no longer referenced.
func (v *Vault) Destroy() {
	if v == nil {
		return
	}
	v.key.Destroy()
}

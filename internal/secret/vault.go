// Package secret は、OpenSSH 接続で使用する資格情報を暗号化して保存する。
// 保存先はワークスペース内の ~/.ssh/sshc/secrets。鍵はマスターパスワード、
// またはパスワードなしモードの端末専用乱数から導出する。
package secret

import (
	"encoding/hex"
	"errors"
	"maps"
	"slices"
	"strings"

	"sshc/internal/envelope"
	"sshc/internal/totp"
	"sshc/internal/validate"
)

// WorkspacePath は、暗号化されたファイルの置き場所。ワークスペースルートからの
// 相対である。エディタに開いてくれと誘うような拡張子を持たず、読めそうに見える
// 名前も持たない。
const WorkspacePath = "sshc/secrets"

var (
	// ErrUnsafeName は、安全な alias ではない alias を拒否する。
	ErrUnsafeName = errors.New("that is not a safe host alias")
	// ErrEmptySecret は空のパスワードを拒否する。プロンプト上では、誤ったものと
	// 区別がつかないからだ。
	ErrEmptySecret = errors.New("the password is empty")
	// ErrUnknownKind は、どちらでもない名前空間を拒否する。
	ErrUnknownKind = errors.New("that is not a credential kind")
	// ErrUnknownCredential は、その名前空間に存在しない名前への参照を拒否する。
	// これは、ホストが鍵のパスフレーズを参照するのを止めている仕組みでも
	// ある。
	ErrUnknownCredential = errors.New("no credential of that kind has that name")
	// ErrCredentialInUse は、まだ何かが指している秘密の削除を拒む。
	ErrCredentialInUse = errors.New("something still uses this credential")
	// ErrInvalidTOTP は、TOTP のセットアップキーまたは provisioning URI が
	// RFC 6238 の設定として解釈できないことを報告する。
	ErrInvalidTOTP = errors.New("the TOTP setup key is invalid")
)

// MinPassphraseLength は、これが受け付ける最短の vault パスフレーズ長。
const MinPassphraseLength = 4

// 利用者が打ち込んで Vault に入れる値の上限。HTTP の入口も同じ値で断る。
const (
	// MaxCredentialNameLength は、共有の認証情報に付ける名前の上限。
	MaxCredentialNameLength = 128
	// MaxPasswordLength は、保存するパスワードと鍵のパスフレーズの上限。
	MaxPasswordLength = 1024
)

// Kind は、資格情報の名前空間を表す。
//
// ホストは KindPassword と KindTOTP のみを、鍵は KindKeyPassphrase のみを参照できる。名前空間が
// ひとつなら、ホストのパスワード選択画面が鍵のパスフレーズを提示できてしまい、それを
// 選べばそのパスフレーズがログインパスワードとしてリモートホストへ送られる。二つに
// 分ければ、それは起こりにくいどころか、表現すること自体が不可能になる。
//
// アカウントパスワードと秘密鍵パスフレーズを別の名前空間に分け、誤送信を防ぐ。
type Kind string

const (
	KindPassword      Kind = "password"
	KindKeyPassphrase Kind = "key_passphrase"
	KindTOTP          Kind = "totp"
	// KindVPN は、VPN プロファイルの秘密である。名前はプロファイル名で、値は
	// その backend が要る秘密をまとめたものである。
	//
	// プロファイルひとつにつき一件にする。秘密は同時に作られ、同時に回し、
	// 同時に消えるので、分けて持つとプロファイルを作る・改名する・消すたびに
	// 複数件の整合を取ることになる。
	//
	// この名前空間は host にも鍵にも割り当てない。資格情報の画面と API にも
	// 現れない。VPN 経路の設定の一部であり、接続先へ送る資格情報ではない。
	KindVPN Kind = "vpn"
)

// ValidKind は、資格情報の画面と API が名指してよい名前空間かを報告する。この
// 集合が決まる唯一の場所なので、ルートとフォームがそれについて食い違うことは
// ありえない。
func ValidKind(kind Kind) bool {
	return kind == KindPassword || kind == KindKeyPassphrase || kind == KindTOTP
}

// storedKind は、vault が値を保存する名前空間かを報告する。
//
// ValidKind より広い。VPN の秘密は vault が保存するが、資格情報として host や
// 鍵へ割り当てるものではないので、画面と API の集合には入れない。
func storedKind(kind Kind) bool {
	return ValidKind(kind) || kind == KindVPN
}

// subjectIsAlias は、kind の subject がホストの alias かを報告する。パスワードと TOTP は
// ホストに割り当て、鍵のパスフレーズは鍵のワークスペース相対のパスに割り当てる。
func subjectIsAlias(kind Kind) bool {
	return kind == KindPassword || kind == KindTOTP
}

// validateSubject は、subject が kind の割り当て先の名前として使えるかを確かめる。
// alias の種類は alias の規則で、鍵のパスフレーズは「空でなく NUL を含まない」で
// 確かめる。
func validateSubject(kind Kind, subject string) error {
	if subjectIsAlias(kind) {
		if validate.Alias(subject) != nil {
			return ErrUnsafeName
		}
		return nil
	}
	if subject == "" || strings.ContainsRune(subject, '\x00') {
		return ErrUnsafeName
	}
	return nil
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
	passwordBindings        map[string]string
	totpBindings            map[string]string
	dedicatedKeyPassphrases map[string]string
}

// newMaps は、秘密と subject の名前空間を用意する。
//
// KindVPN には subject の map を作らない。割り当てる相手を持たない名前空間に
// 空の map を置くと、割り当てられるように見えてしまう。
func newMaps() (map[Kind]map[string]string, map[Kind]map[string]string) {
	return map[Kind]map[string]string{KindPassword: {}, KindKeyPassphrase: {}, KindTOTP: {}, KindVPN: {}},
		map[Kind]map[string]string{KindPassword: {}, KindKeyPassphrase: {}, KindTOTP: {}}
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
	delete(v.passwordBindings, alias)
	v.dedicatedPasswords[alias] = value
	return nil
}

// RemoveDedicatedPassword forgets a connection-owned password. A missing
// alias is already the requested state and is therefore not an error.
func (v *Vault) RemoveDedicatedPassword(alias string) {
	delete(v.dedicatedPasswords, alias)
	delete(v.passwordBindings, alias)
}

// Bind は alias の解決済み接続先を記録する。ホストや ProxyJump の経路が変わると、
// 利用者が割り当てを確認し直すまで秘密は解放されない。
func (v *Vault) Bind(kind Kind, alias, binding string) error {
	if err := validate.Alias(alias); err != nil || !validAuthenticationBinding(binding) {
		return ErrUnsafeName
	}
	if _, ok := v.SecretFor(kind, alias); !ok {
		return ErrUnknownCredential
	}
	bindings := v.bindingsOf(kind)
	if bindings == nil {
		return ErrUnsafeName
	}
	bindings[alias] = binding
	return nil
}

// bindingsOf は、経路に束縛される種類（パスワードと TOTP）の束縛表を返す。
// 他の種類は経路に束縛されないので nil。
func (v *Vault) bindingsOf(kind Kind) map[string]string {
	switch kind {
	case KindPassword:
		if v.passwordBindings == nil {
			v.passwordBindings = map[string]string{}
		}
		return v.passwordBindings
	case KindTOTP:
		if v.totpBindings == nil {
			v.totpBindings = map[string]string{}
		}
		return v.totpBindings
	default:
		return nil
	}
}

func validAuthenticationBinding(binding string) bool {
	if len(binding) != 64 {
		return false
	}
	_, err := hex.DecodeString(binding)
	return err == nil
}

// SetDedicatedKeyPassphrase stores an unlock value owned by exactly one private
// key. It is deliberately separate from named credentials so replacing it
// cannot rotate the passphrase used by any other key.
func (v *Vault) SetDedicatedKeyPassphrase(relativePath, value string) error {
	if err := validateSubject(KindKeyPassphrase, relativePath); err != nil {
		return err
	}
	if value == "" {
		return ErrEmptySecret
	}
	delete(v.subjects[KindKeyPassphrase], relativePath)
	v.dedicatedKeyPassphrases[relativePath] = value
	return nil
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
		key: v.key.Clone(), secrets: secrets, subjects: subjects,
		dedicatedPasswords:      maps.Clone(v.dedicatedPasswords),
		passwordBindings:        maps.Clone(v.passwordBindings),
		totpBindings:            maps.Clone(v.totpBindings),
		dedicatedKeyPassphrases: maps.Clone(v.dedicatedKeyPassphrases),
	}
}

// Set は、名前の下に資格情報を保存する。新規作成か、値の置き換えである。
func (v *Vault) Set(kind Kind, name, value string) error {
	if !storedKind(kind) {
		return ErrUnknownKind
	}
	if !validCredentialName(name) {
		return ErrUnsafeName
	}
	if value == "" {
		return ErrEmptySecret
	}
	if kind == KindTOTP {
		configuration, err := totp.Parse(value)
		if err != nil {
			return ErrInvalidTOTP
		}
		value = configuration.URI()
	}
	v.secrets[kind][name] = value
	return nil
}

// RenameCredential は名前付き資格情報そのものを改名し、すべての参照を追従させる。
// subject の Rename とは別の操作であり、既存の別資格情報を暗黙に上書きしない。
func (v *Vault) RenameCredential(kind Kind, from, to string) error {
	if !storedKind(kind) {
		return ErrUnknownKind
	}
	if !validCredentialName(from) || !validCredentialName(to) {
		return ErrUnsafeName
	}
	value, ok := v.secrets[kind][from]
	if !ok {
		return ErrUnknownCredential
	}
	if from == to {
		return nil
	}
	if _, exists := v.secrets[kind][to]; exists {
		return ErrCredentialAlreadyExists
	}
	delete(v.secrets[kind], from)
	v.secrets[kind][to] = value
	for subject, name := range v.subjects[kind] {
		if name == from {
			v.subjects[kind][subject] = to
		}
	}
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
	if err := validateSubject(kind, subject); err != nil {
		return err
	}
	if _, ok := v.secrets[kind][name]; !ok {
		return ErrUnknownCredential
	}
	switch kind {
	case KindPassword:
		delete(v.dedicatedPasswords, subject)
		delete(v.passwordBindings, subject)
	case KindTOTP:
		delete(v.totpBindings, subject)
	default:
		delete(v.dedicatedKeyPassphrases, subject)
	}
	v.subjects[kind][subject] = name
	return nil
}

// Unassign は subject の参照を忘れる。subject がなくてもエラーではない。
func (v *Vault) Unassign(kind Kind, subject string) {
	delete(v.subjects[kind], subject)
	delete(v.bindingsOf(kind), subject)
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

// BoundFor は、束縛を確認したときの接続先が今も同じ場合だけ秘密を返す。
// パスワードは接続先へ、TOTP の provisioning data も同じ境界で解放される。
func (v *Vault) BoundFor(kind Kind, subject, binding string) (string, bool) {
	stored, ok := v.bindingsOf(kind)[subject]
	if !ok || stored != binding {
		return "", false
	}
	return v.SecretFor(kind, subject)
}

// Rename は、ホストの alias の変更に合わせて、alias を subject とする種類
// （パスワードと TOTP）の参照を新しい名前へ引き継ぐ。ホストの名前変更はこれを
// しなければならず、さもなければ参照は、誰も尋ねない名前の下に暗黙に孤児に
// なる。鍵の subject（パス）の移動は RelocateSubjects が担う。
func (v *Vault) Rename(kind Kind, from, to string) error {
	if !subjectIsAlias(kind) {
		return ErrUnknownKind
	}
	if from == to {
		return nil
	}
	if kind == KindPassword {
		if value, ok := v.dedicatedPasswords[from]; ok {
			if err := validateSubject(kind, to); err != nil {
				return err
			}
			delete(v.dedicatedPasswords, from)
			delete(v.subjects[kind], to)
			v.dedicatedPasswords[to] = value
			v.moveBinding(kind, from, to)
			return nil
		}
	}
	name, ok := v.subjects[kind][from]
	if !ok {
		return nil
	}
	if err := validateSubject(kind, to); err != nil {
		return err
	}
	delete(v.subjects[kind], from)
	// A retired destination may still own a dedicated value. Its binding must
	// never be replaced with the source's while that unrelated value survives.
	if kind == KindPassword {
		delete(v.dedicatedPasswords, to)
	}
	v.subjects[kind][to] = name
	v.moveBinding(kind, from, to)
	return nil
}

// moveBinding は subject の経路束縛を新しい名前へ引き継ぐ。束縛を持たない
// 種類では何もしない。
func (v *Vault) moveBinding(kind Kind, from, to string) {
	bindings := v.bindingsOf(kind)
	if bindings == nil {
		return
	}
	binding, ok := bindings[from]
	delete(bindings, from)
	delete(bindings, to)
	if ok {
		bindings[to] = binding
	}
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
		if err := validateSubject(kind, to); err != nil {
			return false, err
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
	if name == "" || len(name) > MaxCredentialNameLength {
		return false
	}
	return !strings.ContainsAny(name, "\x00\r\n")
}

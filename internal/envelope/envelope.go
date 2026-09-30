// Package envelope は、パスフレーズを使ってブロブを暗号化する。
//
// このパッケージがあるのは、「このバイトを、別のマシンからは読めて他のどこからも
// 読めない場所に置く」という問いに対して、このアプリケーションの二つの機能が
// まったく同じ結果を必要とするからである。すなわち、パスワード保管の vault と、
// リモート同期がアップロードするスナップショットだ。実装が二つあれば、コストの
// 上限も二つ、ヘッダー書式も二つ、追加データを取り違える機会も二つになる。
//
// 書式は意図して自己記述的にしてある。暗号化したブロブは、それを書いたビルドより
// 長く生き、書かれた時点では存在しなかったビルドからも読めなければならないから
// である。
//
//	magic 16 | envelope 1 | kdf 1 | time 4 | memory 4 | threads 1 | saltLen 1 | salt | nonce 12 | AES-256-GCM(…)
//	└───────────────────────── 追加データとして認証される ─────────────────────┘
//
// ヘッダーは AEAD の追加データなので、そのパラメータを小さく書き換えて再生する
// ことはできない。1 バイトでも変えれば、安いコストで鍵を導出する代わりに open が
// 失敗する。
package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"slices"

	"golang.org/x/crypto/argon2"
)

var (
	// ErrWrongPassphrase は、与えられたパスフレーズではブロブを開けなかったことを
	// 報告する。「パスフレーズが違う」と「ブロブが改竄されている」で意図的に同じ
	// エラーにしてある。AES-GCM に両者は区別できず、区別できるふりをすればそれは
	// 当て推量になるからだ。
	ErrWrongPassphrase = errors.New("the passphrase did not open this data")
	// ErrNotAnEnvelope は、そのバイト列がそもそも暗号化したブロブではないと報告する。
	ErrNotAnEnvelope = errors.New("these bytes are not an sshc envelope")
	// ErrUnsupportedVersion は、現行形式ではないブロブを報告する。
	ErrUnsupportedVersion = errors.New("this sshc envelope version is not supported")
	// ErrCostRefused は、このアプリケーションが書くどの envelope にも必要ないほどの
	// 作業量を要求するヘッダーを報告する。
	ErrCostRefused = errors.New("this data demands an unreasonable amount of work to open")
	// ErrWeakPassphrase は、MinPassphraseLength 未満のパスフレーズを拒否する。
	ErrWeakPassphrase = errors.New("the passphrase is too short")
)

// MinPassphraseLength は、Derive が受け付ける最短のパスフレーズ長で、同期鍵のように
// 下限を指定しない封の既定である。別の下限が要る呼び出し側は DeriveWithMinimum に
// 渡す（Vault のマスターパスワードは secret.MinPassphraseLength を使う）。
//
// 求めるのは長さだけで、文字種の要件は課さない。それはユーザーを、覚えられない
// 短いパスフレーズへと追いやるからだ。暗号化したブロブはマシンの外へコピーでき、
// 好きなだけ時間をかけてオフラインで攻撃できる。それを高くつくものにするのは長さである。
const MinPassphraseLength = 12

// Argon2id のパラメータ。すべてのヘッダーに書き込まれるので、あとで引き上げても
// 古いブロブは読めるままで、新しいものだけが強くなる。
const (
	kdfArgon2id      = 1
	defaultTime      = 3
	defaultMemoryKiB = 64 * 1024
	defaultThreads   = 4
	derivedKeyLength = 32
	saltLength       = 16
	nonceLength      = 12
	magicLength      = 16
	envelopeVersion  = 1
)

// コストの上限。
//
// 開くのにどれだけの作業がかかるかはヘッダーが述べており、暗号化したブロブは外から
// やってくる。別のマシンから、バケットから、リストアから。まず妥当だと同意して
// いないパラメータから鍵を導出してよいものは何もない。
//
// これがないと、64 MiB 上で time=65539 を主張するヘッダーは、試行あたり 1 コアで
// おおよそ 90 分を要求し、open は決して戻ってこない。これは仮定の話ではない。
// vault 自身の改竄テストの初回実行が、コストのフィールドの 1 ビットを反転させる
// ことで実際にそうなり、誰かが見に来るまで 5 分間ハングしていた、という実話で
// ある。
//
// 大きなパラメータを拒否することは弱体化ではない。ヘッダーは認証されているので、
// 攻撃者が実際のコストを下げたうえでブロブを開けるようにすることはできない。
// この上限は、終わらせる価値のない作業を始めさせないだけである。
const (
	maxKDFTime      = 16
	maxKDFMemoryKiB = 1 << 20 // 1 GiB
	maxKDFThreads   = 16
	maxSaltLength   = 64
)

// Limits は、envelope が要求してよいパラメータ。
//
// 二組ある。envelope には二種類あり、そのうち自分たちのものは一方だけだからだ。
// このインストールが書いたファイルは Derive が選んだ値を要求する。バケットから
// 取ってきたスナップショットは、それを書いた誰かが選んだ値を要求し、それは別のユーザーが
// 決めた数字である。後者の上限をこちらが書くであろう値の近くに置いてあるので、
// スナップショットが、パスフレーズの誤りが判明する前にこのマシンへ 1 ギガバイトと
// 16 スレッドを費やさせることはできない。
type Limits struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// Accepted は、このインストールが書いた envelope が要求してよい値。Open が使う。
var Accepted = Limits{Time: maxKDFTime, MemoryKiB: maxKDFMemoryKiB, Threads: maxKDFThreads}

// AcceptedFromRemote は、ネットワーク越しに届いた envelope が要求してよい値。
// OpenRemote が使う。Derive が書く値までで、それ以上はない。
var AcceptedFromRemote = Limits{Time: defaultTime, MemoryKiB: defaultMemoryKiB, Threads: defaultThreads}

// DerivationCost は、Derive が新しい鍵に使う Argon2id のコスト。
//
// 製品では書き換えない。書き換えてよいのは、鍵導出の強さではなく同期などの手順を
// 確かめるテストだけである。race detector の下では Argon2id の Go 実装
// （アセンブリの無い arm64 など）が極端に遅く、push と pull のたびに鍵を導出する
// テストが CI の上限を超える。コストはヘッダーに書かれ、開く側はヘッダーを
// 読むので、下げたコストで封をしたブロブも同じ Open で開ける。
var DerivationCost = Limits{Time: defaultTime, MemoryKiB: defaultMemoryKiB, Threads: defaultThreads}

var magic = [magicLength]byte{'s', 's', 'h', '-', 'u', 'i', '-', 'e', 'n', 'v', 'e', 'l', 'o', 'p', 'e', 0}

// Params は、暗号化したブロブひとつ分の鍵導出パラメータ。
type Params struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	Salt    []byte
}

// Key は導出済みの鍵。パスフレーズではなく鍵を保持するのは、変更を暗号化し直すときに
// パスフレーズをもう一度尋ねずに済ませるためである。
type Key struct {
	material []byte
	params   Params
}

// Clone returns an independently owned copy of the derived key. Callers which
// keep two key generations alive at once must not share the backing material:
// destroying a discarded candidate must never erase the live generation.
func (k Key) Clone() Key {
	return Key{
		material: slices.Clone(k.material),
		params: Params{
			Time: k.params.Time, Memory: k.params.Memory, Threads: k.params.Threads,
			Salt: slices.Clone(k.params.Salt),
		},
	}
}

// Destroy best-effort overwrites derived key material before releasing it.
// Go may have copied the bytes elsewhere, so this is not a perfect erasure
// guarantee; it still narrows the window in which a post-lock memory capture
// can recover the deliberately discarded key.
func (k *Key) Destroy() {
	if k == nil {
		return
	}
	clear(k.material)
	k.material = nil
	k.params = Params{}
}

// Derive は、新しいパラメータで passphrase を伸ばして鍵にする。
func Derive(passphrase string) (Key, error) {
	return DeriveWithMinimum(passphrase, MinPassphraseLength)
}

// DeriveWithMinimum applies the caller’s passphrase policy without changing the format.
func DeriveWithMinimum(passphrase string, minimum int) (Key, error) {
	if minimum < 1 || len([]rune(passphrase)) < minimum {
		return Key{}, ErrWeakPassphrase
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return Key{}, err
	}
	cost := DerivationCost
	params := Params{Time: cost.Time, Memory: cost.MemoryKiB, Threads: cost.Threads, Salt: salt}
	return Key{material: derive(passphrase, params), params: params}, nil
}

// MaxConcurrentDerivations は、同時に走ってよい鍵導出の総数。
//
// 鍵導出は意図的に高価であり（数十メガバイトと複数スレッド）アンロック、push、
// pull のいずれもが一回ずつ行う。上限がなければ、タブがいくつか開いたページが
// 一度に何十個も要求し、プロセスは理由もなくギガバイト単位を確保する。remote
// envelope だけはさらに厳しい上限を持つ。ローカルの鍵変更まで、攻撃者が
// 用意した遅い remote envelope の終了待ちにしないため、総数は2件を維持する。
const MaxConcurrentDerivations = 2

// MaxConcurrentRemoteDerivations は、OpenRemote が認証前の remote envelope に同時に
// 費やしてよい鍵導出の数。1件あたりは現在の書き込み値である64 MiBまでに制限される。
const MaxConcurrentRemoteDerivations = 1

var derivations = make(chan struct{}, MaxConcurrentDerivations)
var remoteDerivations = make(chan struct{}, MaxConcurrentRemoteDerivations)

// OnDerive は各導出を包む。同時に何個走っているかを数えるテストのためにあり、
// それ以外の場所では nil である。
var OnDerive func(step func())

func derive(passphrase string, params Params) []byte {
	derivations <- struct{}{}
	defer func() { <-derivations }()

	var key []byte
	step := func() {
		key = argon2.IDKey([]byte(passphrase), params.Salt, params.Time, params.Memory, params.Threads, derivedKeyLength)
	}
	if OnDerive != nil {
		OnDerive(step)
	} else {
		step()
	}
	return key
}

// Seal は key で plaintext を暗号化する。呼び出しごとに新しい nonce を使うので、
// 同じ内容を二度暗号化してもバイト列は異なり、どちらも両者が同じ内容であることを
// 明かさない。
func (k Key) Seal(plaintext []byte) ([]byte, error) {
	if len(k.material) != derivedKeyLength {
		return nil, ErrNotAnEnvelope
	}
	gcm, err := newGCM(k.material)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceLength)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	header := writeHeader(k.params)
	sealed := make([]byte, 0, len(header)+nonceLength+len(plaintext)+gcm.Overhead())
	sealed = append(sealed, header...)
	sealed = append(sealed, nonce...)
	return gcm.Seal(sealed, nonce, plaintext, header), nil
}

// Open は、すでに保持している鍵で sealed を復号する。Seal の鏡像であり、あちらが
// 鍵だけで動くように、こちらも鍵だけで動く。
//
// これは、鍵を保持し、パスフレーズは意図的に保持しない呼び出し側（vault）の
// ためにある。おかげで vault は自分のファイルの隣にもうひとつ暗号化し、ユーザーに
// 再度尋ねることなく読み戻せる。ヘッダーは認証データなので、鍵が合わない場合は
// 別途検出されるのではなくタグの検証に失敗する。
func (k Key) Open(sealed []byte) ([]byte, error) {
	if len(k.material) != derivedKeyLength {
		return nil, ErrNotAnEnvelope
	}
	header, _, rest, err := readHeader(sealed)
	if err != nil {
		return nil, err
	}
	if len(rest) < nonceLength {
		return nil, ErrNotAnEnvelope
	}
	nonce, ciphertext := rest[:nonceLength], rest[nonceLength:]
	gcm, err := newGCM(k.material)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, header)
	if err != nil {
		return nil, ErrWrongPassphrase
	}
	return plaintext, nil
}

// Open は passphrase で sealed を復号し、平文とともに鍵も返す。呼び出し側が
// 導出をやり直さずに暗号化し直せるようにするためである。このインストールが書いた
// envelope のためにあり、Accepted までのコストを払う。
func Open(sealed []byte, passphrase string) ([]byte, Key, error) {
	checked, err := checkWithin(sealed, Accepted)
	if err != nil {
		return nil, Key{}, err
	}
	return checked.open(passphrase)
}

// OpenRemote は、ネットワーク越しに届いた envelope を Open と同じく開く。
//
// ヘッダーのコストを選んだのはそれを書いた誰かなので、AcceptedFromRemote までしか
// 払わない。パスフレーズが合うかは鍵を導いたあとにしか分からないので、同じ入力を
// 並列に投げられても低メモリのマシンの使用量が線形に増えないよう、鍵導出を
// MaxConcurrentRemoteDerivations 件ずつに並べる。上限を超える envelope は、枠を
// 待たずに断る。
func OpenRemote(sealed []byte, passphrase string) ([]byte, Key, error) {
	checked, err := checkWithin(sealed, AcceptedFromRemote)
	if err != nil {
		return nil, Key{}, err
	}
	remoteDerivations <- struct{}{}
	defer func() { <-remoteDerivations }()
	return checked.open(passphrase)
}

// IsEnvelope は、contents が envelope の形をしているかを返す。鍵は使わず、復号も
// しない。このビルドが扱えないバージョンやコストのものも、envelope の形なら真を返す。
// 暗号化する前の平文の文書を、暗号化した文書と見分けるためにある。
func IsEnvelope(contents []byte) bool {
	_, _, _, err := readHeader(contents)
	return !errors.Is(err, ErrNotAnEnvelope)
}

// checkedEnvelope は、ヘッダーを読み、要求するコストを上限と照らし合わせ終えた envelope。
type checkedEnvelope struct {
	header     []byte
	params     Params
	nonce      []byte
	ciphertext []byte
}

// checkWithin は sealed のヘッダーを読み、要求するコストが limits を超えないことを
// 確かめる。鍵を導く前に断るので、上限を超える envelope には何も費やさない。
func checkWithin(sealed []byte, limits Limits) (checkedEnvelope, error) {
	header, params, rest, err := readHeader(sealed)
	if err != nil {
		return checkedEnvelope{}, err
	}
	if params.Time > limits.Time || params.Memory > limits.MemoryKiB || params.Threads > limits.Threads {
		return checkedEnvelope{}, ErrCostRefused
	}
	if len(rest) < nonceLength {
		return checkedEnvelope{}, ErrNotAnEnvelope
	}
	return checkedEnvelope{
		header: header, params: params,
		nonce: rest[:nonceLength], ciphertext: rest[nonceLength:],
	}, nil
}

// open は passphrase から鍵を導いて復号する。
func (e checkedEnvelope) open(passphrase string) ([]byte, Key, error) {
	material := derive(passphrase, e.params)
	published := false
	defer func() {
		if !published {
			clear(material)
		}
	}()
	gcm, err := newGCM(material)
	if err != nil {
		return nil, Key{}, err
	}
	plaintext, err := gcm.Open(nil, e.nonce, e.ciphertext, e.header)
	if err != nil {
		return nil, Key{}, ErrWrongPassphrase
	}
	published = true
	return plaintext, Key{material: material, params: e.params}, nil
}

// MAC は、この鍵から purpose ごとに導いた鍵で message の HMAC-SHA256 を返す。
// 暗号化の鍵をそのまま MAC に使い回さず、用途ごとに HKDF で分けるので、ある用途の
// MAC がほかの用途の鍵や暗号文について何も明かさない。
func (k Key) MAC(purpose string, message []byte) ([]byte, error) {
	if len(k.material) != derivedKeyLength {
		return nil, ErrNotAnEnvelope
	}
	subkey, err := hkdf.Key(sha256.New, k.material, nil, purpose, sha256.Size)
	if err != nil {
		return nil, err
	}
	defer clear(subkey)
	mac := hmac.New(sha256.New, subkey)
	mac.Write(message)
	return mac.Sum(nil), nil
}

func newGCM(material []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(material)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func writeHeader(params Params) []byte {
	header := make([]byte, 0, magicLength+12+len(params.Salt))
	header = append(header, magic[:]...)
	header = append(header, envelopeVersion, kdfArgon2id)
	header = binary.BigEndian.AppendUint32(header, params.Time)
	header = binary.BigEndian.AppendUint32(header, params.Memory)
	header = append(header, params.Threads, byte(len(params.Salt)))
	return append(header, params.Salt...)
}

func readHeader(sealed []byte) (header []byte, params Params, rest []byte, err error) {
	const fixed = magicLength + 12
	if len(sealed) < fixed {
		return nil, Params{}, nil, ErrNotAnEnvelope
	}
	if [magicLength]byte(sealed[:magicLength]) != magic {
		return nil, Params{}, nil, ErrNotAnEnvelope
	}
	if sealed[magicLength] != envelopeVersion {
		return nil, Params{}, nil, ErrUnsupportedVersion
	}
	if sealed[magicLength+1] != kdfArgon2id {
		// 未知の KDF は、壊れたブロブではなく将来のビルドが書いたブロブである。
		return nil, Params{}, nil, ErrUnsupportedVersion
	}
	params.Time = binary.BigEndian.Uint32(sealed[magicLength+2:])
	params.Memory = binary.BigEndian.Uint32(sealed[magicLength+6:])
	params.Threads = sealed[magicLength+10]
	saltLen := int(sealed[magicLength+11])
	if params.Time == 0 || params.Memory == 0 || params.Threads == 0 || saltLen == 0 {
		return nil, Params{}, nil, ErrNotAnEnvelope
	}
	if params.Time > maxKDFTime || params.Memory > maxKDFMemoryKiB ||
		params.Threads > maxKDFThreads || saltLen > maxSaltLength {
		return nil, Params{}, nil, ErrCostRefused
	}
	if len(sealed) < fixed+saltLen {
		return nil, Params{}, nil, ErrNotAnEnvelope
	}
	params.Salt = slices.Clone(sealed[fixed : fixed+saltLen])
	return slices.Clone(sealed[:fixed+saltLen]), params, sealed[fixed+saltLen:], nil
}

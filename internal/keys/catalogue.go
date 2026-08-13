package keys

import (
	"context"
	"path/filepath"

	"sshc/internal/platform"
)

// このプロセス内でその variant を生成しない理由を説明する理由コード。
const ReasonHardwareToken = "hardware_token_required"

// Variant は、ユーザーが要求しうる鍵の種類ひとつ。
type Variant struct {
	Algorithm Algorithm
	Bits      int
	Label     string
	InProcess bool
	Reason    string
}

// Catalogue は、このアプリケーションが生成できる variant の集合。
type Catalogue struct {
	Variants []Variant
	Source   string
}

// generated は、このプロセスが自分で作れる鍵である。
//
// **インストール済みの OpenSSH には尋ねない。** 以前は `ssh -Q key` に
// 「あちらは何に対応しているか」を尋ねていたが、それは答えるべき問いではない。
// ここが並べているのは「ここで生成できる鍵」であり、生成するのは
// GeneratePrivateKey の x/crypto である。
//
// 尋ねていたのは、生成した鍵がその人の `ssh` で使えるかを確かめる代理だった。
// Ed25519・RSA・ECDSA は、対応していない OpenSSH を探す方が難しい。**代理を
// 立てる必要が無い問いに、代理を立てない。**
//
// **この一覧に載っているものは必ず作れなければならない。** 載っているのに
// 作れない項目は、画面のボタンが必ず失敗するということである。
var generated = []Variant{
	{Algorithm: AlgorithmEd25519, Bits: 256, Label: "Ed25519", InProcess: true},
	{Algorithm: AlgorithmRSA, Bits: 2048, Label: "RSA", InProcess: true},
	{Algorithm: AlgorithmRSA, Bits: 3072, Label: "RSA", InProcess: true},
	{Algorithm: AlgorithmRSA, Bits: 4096, Label: "RSA", InProcess: true},
	{Algorithm: AlgorithmECDSA, Bits: 256, Label: "ECDSA", InProcess: true},
	{Algorithm: AlgorithmECDSA, Bits: 384, Label: "ECDSA", InProcess: true},
	{Algorithm: AlgorithmECDSA, Bits: 521, Label: "ECDSA", InProcess: true},
}

// hardware は、ハードウェアトークンが要る鍵である。
//
// **これだけは ssh-keygen が要る。** PIN もタッチも libfido2 も x/crypto に
// 無いので、作るのは OpenSSH のプログラムである。それが見つからない環境では
// 一覧に出さない——押せば必ず失敗するボタンを画面に出さない。
var hardware = []Variant{
	{Algorithm: AlgorithmEd25519SK, Label: "Ed25519 security key", InProcess: false, Reason: ReasonHardwareToken},
	{Algorithm: AlgorithmECDSASK, Bits: 256, Label: "ECDSA security key", InProcess: false, Reason: ReasonHardwareToken},
}

// CatalogueReader は、生成できる鍵の一覧を返す。
//
// **何も実行しない。** ssh-keygen があるかどうかだけを Toolchain に尋ね、
// あればハードウェア鍵の項目を足す。
type CatalogueReader struct {
	Toolchain platform.Toolchain
}

func (reader CatalogueReader) Read(context.Context) Catalogue {
	catalogue := Catalogue{
		Variants: append([]Variant(nil), generated...),
		Source:   "this application",
	}
	if reader.Toolchain == nil {
		return catalogue
	}
	if _, err := reader.Toolchain.KeyGen(); err != nil {
		return catalogue
	}
	catalogue.Variants = append(catalogue.Variants, hardware...)
	return catalogue
}

// HardwareCommand は、ハードウェアに裏打ちされた鍵のためにユーザーが Terminal で
// 実行しなければならない引数リストを、そのまま返す。
//
// このサブシステムが Terminal を起動することは決してない。その段階はロードマップの
// サブシステム 5 が所有する。各要素はシェルの引用を必要としない文字集合に対して
// 検査されるので、表示される行は曖昧さがなく、どの要素もオプションとして読み直され
// えず、ここにあるものがあとで AppleScript やシェルの構文になることもない。
func HardwareCommand(algorithm Algorithm, fileName, comment, sshDirectory string) ([]string, error) {
	var keyType string
	switch algorithm {
	case AlgorithmEd25519SK:
		keyType = "ed25519-sk"
	case AlgorithmECDSASK:
		keyType = "ecdsa-sk"
	default:
		return nil, ErrUnsupportedAlgorithm
	}
	if err := ValidateFileName(fileName); err != nil {
		return nil, err
	}
	// より厳しいルールを使う。このコマンドラインはユーザーが実行するために表示され
	// るので、ここの各引数はシェルへコピーされても壊れないものでなければならない。
	if err := ValidateHardwareComment(comment); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(sshDirectory) {
		return nil, ErrInvalidFileName
	}

	command := []string{"ssh-keygen", "-t", keyType}
	if comment != "" {
		command = append(command, "-C", comment)
	}
	command = append(command, "-f", filepath.Join(sshDirectory, fileName))
	for _, argument := range command {
		if !safeArgumentPattern.MatchString(argument) {
			return nil, ErrInvalidFileName
		}
	}
	return command, nil
}

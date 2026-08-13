package platform

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// askpass ヘルパーを武装させる 5 つの変数。
//
// ここが唯一の一覧である。この文字列を知る場所が二つあると、片方だけを直した
// 日に、ヘルパーは自分に渡されたはずのものを読めなくなる。
const (
	// AskpassAliasVariable は回答が属するホストの名前を保持する。
	//
	// alias は環境変数として渡され、プロンプトから読み取ることは決してしない。
	// OpenSSH のプロンプトが運ぶのは *解決後の* ユーザー名とホスト名であり、
	// パスワードが登録されている名前とは違う。
	AskpassAliasVariable = "SSHC_ASKPASS_ALIAS"
	// AskpassURLVariable は、これを武装した sshc のループバックエンドポイント。
	AskpassURLVariable = "SSHC_ASKPASS_URL"
	// AskpassTokenVariable は、この接続のためのワンタイムトークン。
	AskpassTokenVariable = "SSHC_ASKPASS_TOKEN"
	// AskpassKindVariable は、トークンが回答できる資格情報の種類を保持する。
	AskpassKindVariable = "SSHC_ASKPASS_KIND"
	// AskpassKeyPathVariable は、トークン発行時に選ばれた解決済みの秘密鍵パス。
	AskpassKeyPathVariable = "SSHC_ASKPASS_KEY_PATH"

	// AskpassKindKeyPassphrase は、いま発行される唯一の種類である。
	// 保存済みアカウントパスワードは OpenSSH へ提供されない。
	AskpassKindKeyPassphrase = "key_passphrase"
)

// askpassVariables は、決定した値で必ず置き換える変数である。順序は決定的で、
// 環境の差分を読む検査が並び順で揺れないようにしてある。
var askpassVariables = []string{
	"SSH_ASKPASS", "SSH_ASKPASS_REQUIRE",
	AskpassURLVariable, AskpassTokenVariable, AskpassAliasVariable,
	AskpassKindVariable, AskpassKeyPathVariable,
}

// AskpassCredential は、ひとつの接続を武装させるものの全体である。
//
// 部分的に埋まった値では武装しない。欠けたものがあれば OpenSSH 自身が尋ねる
// 接続になり、それは正常な接続である。
type AskpassCredential struct {
	// Helper は askpass ヘルパーの絶対パス。PATH 経由で解決されるものは受け取らない。
	Helper string
	// URL は、ヘルパーがトークンを引き換えるループバックのエンドポイント。
	URL string
	// Token はこの接続一回限りのもの。
	Token string
	// Kind はトークンが回答できる資格情報の種類。
	Kind string
	// IdentityFile は、トークン発行時に選ばれた解決済みの秘密鍵パス。
	IdentityFile string
	// SSHConfig は、その解決を凍結した設定の中身。
	SSHConfig string
}

// ErrInteractiveProgram は、対話セッションに使えない ssh のパスを拒否する。
var ErrInteractiveProgram = errors.New("the ssh program path must be absolute")

// InteractiveRequest は、対話セッション一回分の要求である。
type InteractiveRequest struct {
	// SSH は ssh の絶対パス。Toolchain が解決したものだけが渡される。
	SSH   string
	Alias string
	// Inherited は継ぐ環境である。これはユーザーが自分で行ったであろう接続なので、
	// 検査が使う MinimalEnvironment ではなくユーザー自身の環境を引き継ぐ。
	Inherited  []string
	Credential AskpassCredential
}

// InteractiveSession は、対話的な ssh 一回分の起動一式である。
type InteractiveSession struct {
	Path string
	// Arguments は argv の残りであり、argv[0] は含まない。
	Arguments []string
	Env       []string
	// Armed は、保存済み鍵パスフレーズの経路が使われるかを報告する。
	Armed bool
	// Notice は、資格情報がありながら武装しなかった理由である。空なら理由は無い。
	// 呼び出し側はこれを表示してよい。秘密は含まれない。
	Notice string
}

// InteractiveSSH は、対話セッション一回分の argv と環境を組み立てる。
//
// コマンドラインは組み立てない。返すのは argv の要素であり、シェルは間に一度も
// 入らない。凍結した設定を作った場合は、それを片付ける関数を第二の戻り値で返す。
// 呼び出し側は子プロセスが終わったあとに必ずそれを呼ぶ。
//
// 保存済み鍵パスフレーズの経路と、素の接続の経路がここで合流しているのは、
// **環境から 5 つの変数を取り除く処理を二箇所に持たないため**である。exec.Cmd は
// 配列をそのまま渡し、getenv はその中で最初に一致したものを返すので、追記方式では
// ユーザーが何年も前にエクスポートした SSH_ASKPASS に負ける——しかも負けながら、
// 保存済み鍵パスフレーズと引き換えられるトークンをそのプログラムに渡してしまう。
// 武装しない接続でも取り除くのは、古い変数が接続を勝手に武装させないためである。
func InteractiveSSH(request InteractiveRequest) (InteractiveSession, func(), error) {
	noCleanup := func() {}
	if err := ValidateAlias(request.Alias); err != nil {
		return InteractiveSession{}, noCleanup, err
	}
	if !filepath.IsAbs(request.SSH) {
		return InteractiveSession{}, noCleanup, ErrInteractiveProgram
	}

	credential := request.Credential
	session := InteractiveSession{Path: request.SSH}
	if credential.Token == "" {
		session.Arguments = []string{"--", request.Alias}
		session.Env = askpassEnvironment(request.Inherited, nil)
		return session, noCleanup, nil
	}

	// 部分的に埋まった資格情報で武装はしない。ヘルパーを PATH 経由で探させれば、
	// 他人が供給しうるプログラムへトークンを渡すことになる。
	switch {
	case credential.Kind != AskpassKindKeyPassphrase:
		session.Notice = "the running sshc returned an unsupported credential kind"
	case !filepath.IsAbs(credential.Helper) || credential.URL == "":
		session.Notice = "the askpass helper could not be located"
	case credential.IdentityFile == "" || credential.SSHConfig == "":
		session.Notice = "the running sshc did not provide a fixed SSH configuration"
	}
	if session.Notice != "" {
		session.Arguments = []string{"--", request.Alias}
		session.Env = askpassEnvironment(request.Inherited, nil)
		return session, noCleanup, nil
	}

	configPath, cleanup, err := FreezeSSHConfig(credential.SSHConfig)
	if err != nil {
		session.Notice = "the fixed SSH configuration could not be written"
		session.Arguments = []string{"--", request.Alias}
		session.Env = askpassEnvironment(request.Inherited, nil)
		return session, noCleanup, nil
	}

	session.Armed = true
	session.Arguments = []string{
		"-F", configPath,
		"-i", credential.IdentityFile,
		"-o", "IdentitiesOnly=yes",
		"--", request.Alias,
	}
	session.Env = askpassEnvironment(request.Inherited, map[string]string{
		"SSH_ASKPASS":          credential.Helper,
		"SSH_ASKPASS_REQUIRE":  "force",
		AskpassURLVariable:     credential.URL,
		AskpassTokenVariable:   credential.Token,
		AskpassAliasVariable:   request.Alias,
		AskpassKindVariable:    credential.Kind,
		AskpassKeyPathVariable: credential.IdentityFile,
	})
	return session, cleanup, nil
}

// askpassEnvironment は、継いだ環境から 5 つの変数を取り除き、決定した値だけを
// 足し直す。decided が nil なら取り除くだけである。
func askpassEnvironment(inherited []string, decided map[string]string) []string {
	ours := make(map[string]bool, len(askpassVariables))
	for _, name := range askpassVariables {
		ours[name] = true
	}

	environment := make([]string, 0, len(inherited)+len(decided))
	for _, entry := range inherited {
		name, _, found := strings.Cut(entry, "=")
		if found && ours[name] {
			continue
		}
		environment = append(environment, entry)
	}
	for _, name := range askpassVariables {
		if value, set := decided[name]; set {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

// FreezeSSHConfig は、ひとつの接続のためだけの設定を私有ディレクトリへ書き、
// 冪等な後始末を返す。
//
// OpenSSH は macOS では -F を読む前に継承した記述子を閉じ、hostname の正規化の
// ためにそのファイルを開き直すことがある。したがってこれは、ssh が終わるまで
// 名前を持ち、開き直せるファイルでなければならない。
func FreezeSSHConfig(contents string) (string, func(), error) {
	directory, err := os.MkdirTemp("", "sshc-connect-*")
	if err != nil {
		return "", func() {}, err
	}
	path := filepath.Join(directory, "config")
	cleaned := false
	cleanup := func() {
		if cleaned {
			return
		}
		cleaned = true
		_ = os.Remove(path)
		_ = os.Remove(directory)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	if _, err := io.WriteString(file, contents); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

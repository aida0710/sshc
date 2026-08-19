// Package remotekey は、リモートアカウントの authorized_keys ファイルに公開鍵を
// インストールする。
//
// リモートで実行するコマンドはパッケージ定数である。鍵は標準入力を通って渡り、
// 固定のルーチンがそれをシェル変数へ読み込む。したがって呼び出し側の入力が、
// コマンドライン・シェル文字列・ヒアドキュメントに差し込まれることは決してない。
package remotekey

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"sshc/internal/effective"
	"sshc/internal/knownhosts"
	"sshc/internal/platform"
	"sshc/internal/sshclient"
)

const (
	// ProbeMarker は、POSIX シェルが返してこなければならない語。
	ProbeMarker = "sshc-posix-shell"
	// ProbeCommand は、リモートアカウントに POSIX シェルがあるかを判定するための
	// 固定コマンド。既知の語をひとつだけ出力し、それ以外は何も出さない。
	ProbeCommand = `printf '%s\n' sshc-posix-shell`
	// RemotePath は、このパッケージが追記するファイル。
	RemotePath = "~/.ssh/authorized_keys"

	// 登録の結果。
	RegistrationAdded    = "added"
	RegistrationExisting = "already_present"
)

// Routine はリモートで走るプログラムの全体。呼び出し側の入力を一切含まない。
//
// 鍵は標準入力で届き "$key" に読み込まれる。grep -x -F は行全体をリテラルとして
// 比較するので、すでに存在する鍵が重複することはない。権限は何かを書く前に
// 締められる。
const Routine = `set -e
umask 077
key=$(cat)
case "$key" in
  ssh-*|ecdsa-*|sk-*) ;;
  *) echo "sshc: unsupported key" >&2; exit 3 ;;
esac
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
touch "$HOME/.ssh/authorized_keys"
chmod 600 "$HOME/.ssh/authorized_keys"
if grep -qxF "$key" "$HOME/.ssh/authorized_keys"; then
  echo "sshc: already-present"
  exit 0
fi
printf '%s\n' "$key" >> "$HOME/.ssh/authorized_keys"
echo "sshc: added"
`

var (
	ErrInvalidPublicKey  = errors.New("public key must be exactly one valid OpenSSH public key line")
	ErrUnsupportedRemote = errors.New("this remote environment does not provide the POSIX shell this operation needs")
	ErrNotAcknowledged   = errors.New("connecting would run a configured command that has not been acknowledged")
)

// publicKeyPattern が受け付ける行はひとつの形だけ。既知のアルゴリズム名、base64
// のかたまり、そして制御文字を含まない任意のコメントである。
var publicKeyPattern = regexp.MustCompile(
	`^(ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp256|ecdsa-sha2-nistp384|ecdsa-sha2-nistp521|sk-ssh-ed25519@openssh\.com|sk-ecdsa-sha2-nistp256@openssh\.com) ([A-Za-z0-9+/]+={0,3})( [^\x00-\x1f\x7f]*)?$`)

// PublicKey は、呼び出し側が選んだ鍵。選ぶのは鍵 vault サブシステムで、この
// パッケージが必要とするのは、その出所のファイルと正確な一行だけである。
type PublicKey struct {
	Path string
	Line string
}

// ParsePublicKey は公開鍵の一行を検証し、そのフィンガープリントを返す。
func ParsePublicKey(line string) (PublicKey, string, error) {
	trimmed := strings.TrimRight(line, "\n")
	if strings.ContainsAny(trimmed, "\n\r") || !publicKeyPattern.MatchString(trimmed) {
		return PublicKey{}, "", ErrInvalidPublicKey
	}
	fields := strings.Fields(trimmed)
	fingerprint, err := knownhosts.Fingerprint(fields[1])
	if err != nil {
		return PublicKey{}, "", ErrInvalidPublicKey
	}
	return PublicKey{Line: trimmed}, fingerprint, nil
}

// ManualSteps は、このパッケージが自動化しないリモートに対して表示する手順。
// 何をすべきかを説明するだけで、何かを実行することは決してない。
var ManualSteps = []string{
	"Open a session to the host yourself and check which shell the account uses.",
	"Create ~/.ssh with mode 700 and ~/.ssh/authorized_keys with mode 600 if they do not exist.",
	"Append the public key line shown above to ~/.ssh/authorized_keys as a single line.",
	"Confirm the file still contains one key per line and that no key was split or duplicated.",
}

// Plan は、リモートホスト上で何かが走る前にユーザーが確認する内容。
type Plan struct {
	Alias       string
	User        string
	Hostname    string
	Port        string
	ValuesFrom  string
	Fingerprint string
	KeyPath     string
	KeyLine     string
	RemotePath  string
	Routine     string
	Supported   bool
	Manual      []string
}

// Evidence は、確認画面が表示したリモート登録計画全体の安定した
// ダイジェスト。実行可能ディレクティブだけでなく、接続先・ユーザー・鍵・
// 設置先も含むため、確認後に設定や入力のどれかが変わればトークンは無効になる。
func (p Plan) Evidence(executableEvidence string, configSnapshot []byte) string {
	configSum := sha256.Sum256(configSnapshot)
	payload, _ := json.Marshal(struct {
		Plan               Plan   `json:"plan"`
		ExecutableEvidence string `json:"executableEvidence"`
		ConfigEvidence     string `json:"configEvidence"`
	}{
		Plan: p, ExecutableEvidence: executableEvidence,
		ConfigEvidence: hex.EncodeToString(configSum[:]),
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Result は、完了した登録ひとつ分。
type Result struct {
	Outcome   string
	ExitCode  int
	Stderr    string
	Truncated bool
}

// Service は、リモート登録を実行する。
//
// **外部プログラムは起こさない。** リモートで走らせる 1 本のコマンドは、この
// プロセスが開いた exec チャンネルの上を通る。凍結した設定ファイルも要らない
// ——接続に使う値を決めるのはこのアプリケーション自身である。
type Service struct {
	// Resolve は、alias ひとつ分の接続を決める。**登録一回につき一度だけ呼ばれる。**
	//
	// 一度なのは、probe と routine が同じ相手へ届かなければならないからである。
	// 二度解決すると、その間に設定を書き換えた者が二本目の行き先を変えられる。
	// かつては設定を凍結してファイルへ書くことで同じ性質を守っていた。
	Resolve func(alias string) (sshclient.Target, error)
	// Run は、決まった接続でコマンドを 1 本走らせる。nil なら登録はできない。
	Run     func(ctx context.Context, target sshclient.Target, command string, stdin []byte) (sshclient.Output, error)
	Timeout time.Duration
}

// ErrNoRunner は、リモートで走らせる手段が配線されていないことを報告する。
var ErrNoRunner = errors.New("no remote command runner is available")

// Plan は、どこにも接触せずに変更内容を説明する。
func (s Service) Plan(alias string, key PublicKey, fingerprint, user, hostname, port, valuesFrom string) Plan {
	return Plan{
		Alias:       alias,
		User:        user,
		Hostname:    hostname,
		Port:        port,
		ValuesFrom:  valuesFrom,
		Fingerprint: fingerprint,
		KeyPath:     key.Path,
		KeyLine:     key.Line,
		RemotePath:  RemotePath,
		Routine:     Routine,
		Supported:   true,
		Manual:      ManualSteps,
	}
}

// Register はリモートのシェルを調べ、そのうえで鍵をインストールする。
func (s Service) Register(ctx context.Context, report effective.Report, configSnapshot []byte, alias string, key PublicKey, acknowledged bool) (Result, error) {
	if err := platform.ValidateAlias(alias); err != nil {
		return Result{}, err
	}
	if _, _, err := ParsePublicKey(key.Line); err != nil {
		return Result{}, err
	}
	if len(report.Unavoidable()) > 0 && !acknowledged {
		return Result{}, ErrNotAcknowledged
	}
	if s.Run == nil || s.Resolve == nil {
		return Result{}, ErrNoRunner
	}

	// **行き先は一度だけ決める。** probe と routine は同じ相手へ届かなければ
	// ならない。
	target, err := s.Resolve(alias)
	if err != nil {
		return Result{}, err
	}

	probe, err := s.Run(ctx, target, ProbeCommand, nil)
	if err != nil {
		return Result{}, err
	}
	if probe.ExitCode != 0 || strings.TrimSpace(string(probe.Stdout)) != ProbeMarker {
		return Result{}, ErrUnsupportedRemote
	}

	// **公開鍵は stdin を通る。argv には決して乗らない。**
	output, err := s.Run(ctx, target, Routine, []byte(key.Line+"\n"))
	if err != nil {
		return Result{}, err
	}
	result := Result{
		ExitCode:  output.ExitCode,
		Stderr:    string(output.Stderr),
		Truncated: output.Truncated,
	}
	switch {
	case strings.Contains(string(output.Stdout), "sshc: already-present"):
		result.Outcome = RegistrationExisting
	case strings.Contains(string(output.Stdout), "sshc: added"):
		result.Outcome = RegistrationAdded
	default:
		return result, ErrUnsupportedRemote
	}
	return result, nil
}

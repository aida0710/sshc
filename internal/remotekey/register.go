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

	"sshc/internal/effective"
	"sshc/internal/knownhosts"
	"sshc/internal/sshclient"
	"sshc/internal/validate"
)

const (
	// ProbeMarker は、POSIX シェルが返してこなければならない語。
	ProbeMarker = "sshc-posix-shell"
	// ProbeCommand は、リモートアカウントに POSIX シェルがあるかを判定するための
	// 固定コマンド。既知の語をひとつだけ出力し、それ以外は何も出さない。
	ProbeCommand = `printf '%s\n' sshc-posix-shell`
	// RemotePath は、このパッケージが追記するファイル。
	RemotePath = "~/.ssh/authorized_keys"
	// RegistrationLimit は、登録全体（probe と登録の 2 回の接続、認証、2 本の
	// コマンド）が終わるまでの上限である。ConnectTimeout は認証の段階に掛からず、
	// 登録の画面には止める操作が無い。認証の返事をしない相手でも画面を待たせ続け
	// ないよう、ここで打ち切る。既定の ConnectTimeout（sshclient.DefaultTimeout）で
	// 2 回つなぎ、ssh-agent の確認（Touch ID）を待っても収まるよう、その 4 倍にする。
	RegistrationLimit = 4 * sshclient.DefaultTimeout

	// 登録の結果。
	RegistrationAdded    = "added"
	RegistrationExisting = "already_present"
)

// Routine はリモートで走るプログラムの全体。呼び出し側の入力を一切含まない。
//
// 鍵は標準入力で届き "$key" に読み込まれる。すでに登録済みかどうかは、行全体では
// なく「鍵の種類 + base64 本体」の並びで判定する。コメントや options だけが違う
// 既存行も同じ鍵として扱い、制限付きの行の後ろに制限のない行を足さない。
// 空白とタブのあとの # で始まる行は sshd が読まないので判定から外す。無効にする
// ためにコメントアウトした同じ鍵があっても、登録済みとは答えずに追記する。
// 古い mawk は [[:space:]] を解さないので、sshd と同じ空白とタブを並べて書く。
// 末尾に改行のないファイルへそのまま追記すると最終行に連結されて旧鍵まで壊れる
// ので、ssh-copy-id と同じく先に改行を補う。権限は何かを書く前に締められる。
const Routine = `set -e
umask 077
key=$(cat)
case "$key" in
  ssh-*|ecdsa-*|sk-*) ;;
  *) echo "sshc: unsupported key" >&2; exit 3 ;;
esac
key_type=${key%% *}
key_rest=${key#* }
key_blob=${key_rest%% *}
file="$HOME/.ssh/authorized_keys"
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
touch "$file"
chmod 600 "$file"
if SSHC_KEY_TYPE=$key_type SSHC_KEY_BLOB=$key_blob awk '
  /^[ \t]*#/ { next }
  { for (i = 1; i < NF; i++) if ($i == ENVIRON["SSHC_KEY_TYPE"] && $(i + 1) == ENVIRON["SSHC_KEY_BLOB"]) found = 1 }
  END { exit !found }' "$file"; then
  echo "sshc: already-present"
  exit 0
fi
if [ -s "$file" ] && [ -n "$(tail -c 1 "$file")" ]; then
  printf '\n' >> "$file"
fi
printf '%s\n' "$key" >> "$file"
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

// Service は、プロセス内 SSH 接続でリモート登録を実行する。
type Service struct {
	// Resolve は probe と登録処理で同じ接続先を使うため、登録ごとに一度だけ呼ぶ。
	Resolve func(alias string) (sshclient.Target, error)
	// Run は、決まった接続でコマンドを 1 本走らせる。nil なら登録はできない。
	Run func(ctx context.Context, target sshclient.Target, command sshclient.Command) (sshclient.Output, error)
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
	if err := validate.Alias(alias); err != nil {
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

	// probe と登録処理で同じ接続先を使う。
	target, err := s.Resolve(alias)
	if err != nil {
		return Result{}, err
	}

	ctx, stop := context.WithTimeout(ctx, RegistrationLimit)
	defer stop()

	probe, err := s.Run(ctx, target, sshclient.Command{Line: ProbeCommand})
	if err != nil {
		return Result{}, err
	}
	if probe.ExitCode != 0 || strings.TrimSpace(string(probe.Stdout)) != ProbeMarker {
		return Result{}, ErrUnsupportedRemote
	}

	// 公開鍵はコマンド引数ではなく標準入力で渡す。
	output, err := s.Run(ctx, target, sshclient.Command{Line: Routine, Stdin: []byte(key.Line + "\n")})
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

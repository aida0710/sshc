package vpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"sshc/internal/commandconn"
	"sshc/internal/connectionlog"
)

var (
	// ErrDockerMissing は、docker のコマンドが見つからないことを表す。この機能は
	// Docker が入っているマシンでだけ使える。
	ErrDockerMissing = errors.New("docker is not installed")
	// ErrDockerNotRunning は、docker のコマンドはあるが、Docker が動いていない
	// ことを表す（Docker Desktop を起動していない、など）。
	ErrDockerNotRunning = errors.New("docker is not running")
	// ErrImageBuild は、VPNコンテナのイメージを作れなかったことを表す。
	ErrImageBuild = errors.New("the vpn container image could not be built")
	// ErrTunnelDevice は、backendが要るデバイスがこの機械に無いことを表す。
	ErrTunnelDevice = errors.New("the tunnel device is not available")
)

// maxDockerOutputBytes は、docker の出力を読む上限である。壊れた出力で
// engineの memory を埋めない。
const maxDockerOutputBytes = 1 << 20

// dockerCommand は、docker の実行ファイルひとつと、それを起動する環境である。
//
// Docker API のclientを持たない。engineはdocker socketを直接叩かず、利用者の
// マシンにすでにある docker が使える範囲だけを使う。
type dockerCommand struct {
	path string
	// summary は、docker info で分かった Docker の様子である（OS とアーキテクチャ、
	// 版、どの製品か）。接続ログに出す。
	summary string
	// environment は、docker を起動するときの環境である。PATH はログインシェルの
	// ものにしてある。docker は認証情報の補助（docker-credential-desktop）や
	// buildx を PATH から探す。
	environment []string
}

// Environment は、docker を探して起動するための環境を返す。
//
// launchd や systemd が起動した engine の PATH は短く、Docker Desktop の
// /usr/local/bin も Homebrew の /opt/homebrew/bin も含まない。engine は、
// ログインシェルの PATH を移した環境をここに渡す。nil なら engine の環境を使う。
type Environment func(context.Context) ([]string, error)

// findDocker は、variables の PATH で使える docker を探す。variables が nil なら
// engine の環境で探す。
//
// 見つからない場合と、Docker が動いていない場合を分けて返す。利用者がすることが
// 違う（インストールするか、起動するか）。
func findDocker(ctx context.Context, variables []string) (dockerCommand, error) {
	if variables == nil {
		variables = os.Environ()
	}
	path, err := lookPathIn("docker", pathVariable(variables))
	if err != nil {
		return dockerCommand{}, fmt.Errorf("%w: %w", ErrDockerMissing, err)
	}
	command := dockerCommand{path: path, environment: variables}
	summary, err := command.output(ctx, "info", "--format",
		"{{.OSType}}/{{.Architecture}}、Docker {{.ServerVersion}}、{{.OperatingSystem}}")
	if err != nil {
		return dockerCommand{}, fmt.Errorf("%w: %s: %w", ErrDockerNotRunning, path, err)
	}
	command.summary = strings.TrimSpace(summary)
	return command, nil
}

// pathVariable は、環境のうち PATH の値を返す。後に書かれたものが勝つ。
func pathVariable(variables []string) string {
	for index := len(variables) - 1; index >= 0; index-- {
		if value, found := strings.CutPrefix(variables[index], "PATH="); found {
			return value
		}
	}
	return ""
}

// lookPathIn は、path に並ぶディレクトリから name の実行ファイルを探す。
//
// exec.LookPath は engine 自身の PATH しか見ない。docker を起動する環境の PATH で
// 探さないと、起動する環境と探した環境が食い違う。
func lookPathIn(name, path string) (string, error) {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, directory := range filepath.SplitList(path) {
		if !filepath.IsAbs(directory) {
			// 相対の要素は、engine の作業ディレクトリ次第で別のものを指す。
			continue
		}
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && executable(info) {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

// executable は、実行できるファイルかを返す。Windows には実行の許可の bit が無い。
func executable(info os.FileInfo) bool {
	return runtime.GOOS == "windows" || info.Mode()&0o111 != 0
}

func (command dockerCommand) output(ctx context.Context, arguments ...string) (string, error) {
	return command.run(ctx, dockerCall{arguments: arguments})
}

// combined は、標準出力と標準エラーを、書かれた順に合わせて返す。
//
// docker logs は、コンテナの標準出力をこちらの標準出力へ、標準エラーをこちらの
// 標準エラーへ流す。agent は失敗の理由を標準エラーへ書くので、片方だけを読むと、
// いちばん知りたい行が落ちる。別々に読んでつなぐと、行の前後が入れ替わる。
func (command dockerCommand) combined(ctx context.Context, arguments ...string) (string, error) {
	return command.run(ctx, dockerCall{arguments: arguments, mergeOutput: true})
}

// outputWithInput は、標準入力を渡して docker を実行する。秘密はここを通る。
// 引数にも環境変数にも秘密を置かない。
func (command dockerCommand) outputWithInput(ctx context.Context, input string, arguments ...string) (string, error) {
	return command.run(ctx, dockerCall{arguments: arguments, input: input})
}

// probe は、「無い」ことも答えのひとつである問い合わせ（そのコンテナやイメージが
// あるか）を実行する。無ければ present を false にして、失敗としては返さない。
func (command dockerCommand) probe(ctx context.Context, arguments ...string) (output string, present bool, err error) {
	output, err = command.run(ctx, dockerCall{arguments: arguments, absentIsAnswer: true})
	if isAbsent(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return output, true, nil
}

// isAbsent は、docker の失敗が「その名前のコンテナ（イメージ）は無い」だったかを
// 返す。
//
// それ以外の失敗（daemon が応えない、など）を「無い」と読むと、無いはずの名前で
// コンテナを作りに行き、名前の衝突という分かりにくい失敗になる。
func isAbsent(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "No such container") || strings.Contains(message, "No such object") ||
		strings.Contains(message, "No such image")
}

// stream は、docker を起動し、その標準入出力を接続として返す。
//
// watch には docker の標準エラーが写る。接続先へ繋がったかどうかを読むのに使う。
func (command dockerCommand) stream(watch *connectWatch, arguments ...string) (*commandconn.Conn, error) {
	process := exec.Command(command.path, arguments...)
	process.Env = command.environment
	process.Stderr = watch
	return commandconn.Start(process, "docker "+strings.Join(arguments, " "))
}

// maxDescribedArgumentBytes は、接続ログに出す docker の引数の長さの上限である。
const maxDescribedArgumentBytes = 400

// describeArguments は、docker の引数を接続ログに出す形にする。秘密は引数に
// 載せない決まりなので、そのまま出してよい。
func describeArguments(arguments []string) string {
	described := strings.Join(arguments, " ")
	if len(described) > maxDescribedArgumentBytes {
		described = described[:maxDescribedArgumentBytes] + "…"
	}
	return described
}

// dockerCall は、docker を1回実行するときの指定である。
type dockerCall struct {
	arguments []string
	// input は、標準入力に渡す内容である。
	input string
	// mergeOutput は、標準エラーを標準出力と同じ所へ、書かれた順に集める。
	mergeOutput bool
	// absentIsAnswer は、対象が無いという失敗を問い合わせの答えとして扱い、接続ログに
	// 失敗と書かない。
	absentIsAnswer bool
}

// run は、docker を1回実行し、標準出力を返す。失敗したときは、標準エラーを
// エラーの文に含める。
func (command dockerCommand) run(ctx context.Context, call dockerCall) (output string, err error) {
	started := time.Now()
	defer func() { sayRun(ctx, call, time.Since(started), err) }()
	process := exec.CommandContext(ctx, command.path, call.arguments...)
	process.Env = command.environment
	var stdout, stderr bytes.Buffer
	process.Stdout = &limitedWriter{writer: &stdout, remaining: maxDockerOutputBytes}
	process.Stderr = &limitedWriter{writer: &stderr, remaining: maxDockerOutputBytes}
	if call.mergeOutput {
		// 同じ書き先を渡すと、exec は1本のパイプで受けるので、書かれた順が保たれる。
		process.Stderr = process.Stdout
	}
	if call.input != "" {
		process.Stdin = strings.NewReader(call.input)
	}
	if err := process.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if call.mergeOutput {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail == "" {
			return "", err
		}
		return "", fmt.Errorf("%s: %w", detail, err)
	}
	return stdout.String(), nil
}

// sayRun は、実行したコマンドと掛かった時間を、接続ログの debug3 に書く。失敗した
// ときは、docker の出力も書く。
func sayRun(ctx context.Context, call dockerCall, elapsed time.Duration, err error) {
	described := describeArguments(call.arguments)
	elapsed = connectionlog.Elapsed(elapsed)
	switch {
	case err == nil:
		connectionlog.Say(ctx, connectionlog.Full, "docker %s（%s）", described, elapsed)
	case call.absentIsAnswer && isAbsent(err):
		connectionlog.Say(ctx, connectionlog.Full, "docker %s（%s）：ありません", described, elapsed)
	default:
		connectionlog.Say(ctx, connectionlog.Full, "docker %s は失敗しました（%s）：", described, elapsed)
		sayOutput(ctx, connectionlog.Full, err.Error())
	}
}

// limitedWriter は、上限まで書いたら黙って捨てる。
type limitedWriter struct {
	writer    *bytes.Buffer
	remaining int
}

func (writer *limitedWriter) Write(payload []byte) (int, error) {
	if writer.remaining <= 0 {
		return len(payload), nil
	}
	if len(payload) > writer.remaining {
		writer.writer.Write(payload[:writer.remaining])
		writer.remaining = 0
		return len(payload), nil
	}
	writer.writer.Write(payload)
	writer.remaining -= len(payload)
	return len(payload), nil
}

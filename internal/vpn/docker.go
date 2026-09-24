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

	"sshc/internal/commandconn"
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
	if _, err := command.output(ctx, "info", "--format", "{{.OSType}}"); err != nil {
		return dockerCommand{}, fmt.Errorf("%w: %w", ErrDockerNotRunning, err)
	}
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
	output, _, err := command.run(ctx, "", arguments...)
	return output, err
}

// combined は、標準出力と標準エラーを合わせて返す。
//
// docker logs は、コンテナの標準出力をこちらの標準出力へ、標準エラーをこちらの
// 標準エラーへ流す。agent は失敗の理由を標準エラーへ書くので、片方だけを読むと、
// いちばん知りたい行が落ちる。
func (command dockerCommand) combined(ctx context.Context, arguments ...string) (string, error) {
	output, errorOutput, err := command.run(ctx, "", arguments...)
	if err != nil {
		return "", err
	}
	if errorOutput == "" {
		return output, nil
	}
	if output == "" {
		return errorOutput, nil
	}
	return output + "\n" + errorOutput, nil
}

// outputWithInput は、標準入力を渡して docker を実行する。秘密はここを通る。
// 引数にも環境変数にも秘密を置かない。
func (command dockerCommand) outputWithInput(ctx context.Context, input string, arguments ...string) (string, error) {
	output, _, err := command.run(ctx, input, arguments...)
	return output, err
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

// run は、docker を1回実行し、標準出力と標準エラーを別々に返す。
func (command dockerCommand) run(
	ctx context.Context, input string, arguments ...string,
) (output, errorOutput string, err error) {
	process := exec.CommandContext(ctx, command.path, arguments...)
	process.Env = command.environment
	var stdout, stderr bytes.Buffer
	process.Stdout = &limitedWriter{writer: &stdout, remaining: maxDockerOutputBytes}
	process.Stderr = &limitedWriter{writer: &stderr, remaining: maxDockerOutputBytes}
	if input != "" {
		process.Stdin = strings.NewReader(input)
	}
	if err := process.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return "", "", err
		}
		return "", "", fmt.Errorf("%s: %w", detail, err)
	}
	return stdout.String(), strings.TrimSpace(stderr.String()), nil
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

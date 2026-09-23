package vpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var (
	// ErrDockerMissing は、Dockerを使えないことを表す。この機能はDockerが
	// 動いている機械でだけ使える。
	ErrDockerMissing = errors.New("docker is not available")
	// ErrImageBuild は、VPNコンテナのイメージを作れなかったことを表す。
	ErrImageBuild = errors.New("the vpn container image could not be built")
	// ErrTunnelDevice は、backendが要るデバイスがこの機械に無いことを表す。
	ErrTunnelDevice = errors.New("the tunnel device is not available")
)

// maxDockerOutputBytes は、docker の出力を読む上限である。壊れた出力で
// engineの memory を埋めない。
const maxDockerOutputBytes = 1 << 20

// dockerCommand は、docker の実行ファイルひとつである。
//
// Docker API のclientを持たない。engineはdocker socketを直接叩かず、利用者の
// 機械にすでにある docker が使える範囲だけを使う。
type dockerCommand struct {
	path string
}

// findDocker は、使える docker を探す。見つからない場合と、daemonへ繋がらない
// 場合を区別せずに ErrDockerMissing で返す。どちらも利用者がすることは同じである。
func findDocker(ctx context.Context) (dockerCommand, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return dockerCommand{}, fmt.Errorf("%w: %w", ErrDockerMissing, err)
	}
	command := dockerCommand{path: path}
	if _, err := command.output(ctx, "info", "--format", "{{.OSType}}"); err != nil {
		return dockerCommand{}, fmt.Errorf("%w: %w", ErrDockerMissing, err)
	}
	return command, nil
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

// run は、docker を1回実行し、標準出力と標準エラーを別々に返す。
func (command dockerCommand) run(
	ctx context.Context, input string, arguments ...string,
) (output, errorOutput string, err error) {
	process := exec.CommandContext(ctx, command.path, arguments...)
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

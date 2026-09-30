package vpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"sshc/internal/commandconn"
	"sshc/internal/connectionlog"
	"sshc/internal/platform"
	"sshc/internal/platform/process"
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
	// バージョン、どの製品か）。接続ログに出す。
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

// dockerInfoTimeout は、docker info の答えを待つ上限である。起動の途中の Docker Desktop
// では docker info が長く返らないことがある。そのあいだ、経路の起動と一覧と停止を待たせ
// 続けない。動いている daemon なら1秒ほどで答える。
const dockerInfoTimeout = 20 * time.Second

// findDocker は、variables の PATH で使える docker を探す。variables が nil なら
// engine の環境で探す。
//
// 見つからない場合と、Docker が動いていない場合を分けて返す。利用者がすることが
// 違う（インストールするか、起動するか）。
func findDocker(ctx context.Context, variables []string) (dockerCommand, error) {
	if variables == nil {
		variables = os.Environ()
	}
	searched, _ := platform.LookupEnvironment(variables, "PATH")
	path, err := lookPathIn("docker", searched)
	if err != nil {
		return dockerCommand{}, fmt.Errorf("%w: %w", ErrDockerMissing, err)
	}
	command := dockerCommand{path: path, environment: variables}
	asking, cancel := context.WithTimeout(ctx, dockerInfoTimeout)
	defer cancel()
	summary, err := command.output(asking, "info", "--format",
		"{{.OSType}}/{{.Architecture}}、Docker {{.ServerVersion}}、{{.OperatingSystem}}")
	if err != nil {
		return dockerCommand{}, fmt.Errorf("%w: %s: %w", ErrDockerNotRunning, path, err)
	}
	command.summary = strings.TrimSpace(summary)
	return command, nil
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
//
// コンテナのログにはシークレットが混じりうるので、失敗したときも出力を接続ログへ
// 写さない。出力は *dockerFailure で返し、呼び出し側が伏せてから使う。
func (command dockerCommand) combined(ctx context.Context, arguments ...string) (string, error) {
	return command.run(ctx, dockerCall{arguments: arguments, mergeOutput: true, callerShowsFailure: true})
}

// outputWithInput は、標準入力を渡して docker を実行する。秘密はここを通る。
// 引数にも環境変数にも秘密を置かない。
func (command dockerCommand) outputWithInput(ctx context.Context, input string, arguments ...string) (string, error) {
	return command.run(ctx, dockerCall{arguments: arguments, input: input})
}

// build は、directory からイメージ tag を作る。eachLine は、docker build が書いた行を
// 書かれるたびに受け取る。失敗したときの出力は、呼び出し側が debug2 に写すので、
// ここでは debug3 に重ねて書かない。
func (command dockerCommand) build(ctx context.Context, tag, directory string, eachLine func(line string)) error {
	_, err := command.run(ctx, dockerCall{
		arguments: []string{"build", "--tag", tag, directory}, callerShowsFailure: true, eachLine: eachLine,
	})
	return err
}

// probe は、「無い」ことも答えのひとつである問い合わせ（そのコンテナやイメージが
// あるか）を実行する。無ければ present を false にして、失敗としては返さない。
// subject は、問い合わせた相手の種類（「コンテナ」など）で、無かったときに接続ログへ
// 書く文に使う。
func (command dockerCommand) probe(
	ctx context.Context, subject string, arguments ...string,
) (output string, present bool, err error) {
	output, err = command.run(ctx, dockerCall{arguments: arguments, absentSubject: subject})
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
	child := exec.Command(command.path, arguments...)
	child.Env = command.environment
	child.Stderr = watch
	return commandconn.Start(child, "docker "+strings.Join(arguments, " "))
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
	// eachLine は、docker が標準出力と標準エラーに書いた行を、書かれるたびに受け取る。
	// nil なら渡さない。
	eachLine func(line string)
	// callerShowsFailure は、失敗したときの出力を呼び出し側が接続ログに書くことを
	// 表す。ここでは失敗したことだけを書き、同じ出力を重ねない。
	callerShowsFailure bool
	// absentSubject は、問い合わせた相手の種類である。空でなければ、相手が無いという
	// 失敗を問い合わせの答えとして扱い、接続ログに失敗と書かない。
	absentSubject string
}

// dockerWaitDelay は、キャンセルで docker を止めたあと、出力のパイプが閉じるのを待つ
// 上限である。docker はプロセスグループごと止めるので、docker build が起動した
// docker-buildx のような子もすぐ終わる。グループの外へ出たプロセスがパイプを持って
// いても、経路の起動と停止をそれ以上待たせない。
const dockerWaitDelay = 2 * time.Second

// run は、docker を1回実行し、標準出力を返す。失敗したときは、標準エラーを
// エラーの文に含める。
func (command dockerCommand) run(ctx context.Context, call dockerCall) (output string, err error) {
	started := time.Now()
	defer func() { sayRun(ctx, call, time.Since(started), err) }()
	child := exec.CommandContext(ctx, command.path, call.arguments...)
	child.Env = command.environment
	process.KillGroupOnCancel(child, dockerWaitDelay)
	var stdout, stderr bytes.Buffer
	child.Stdout = &limitedWriter{writer: &stdout, remaining: maxDockerOutputBytes}
	child.Stderr = &limitedWriter{writer: &stderr, remaining: maxDockerOutputBytes}
	if call.eachLine != nil {
		splitter := &lineSplitter{emit: call.eachLine}
		stdoutLines, stderrLines := splitter.stream(), splitter.stream()
		child.Stdout = io.MultiWriter(child.Stdout, stdoutLines)
		child.Stderr = io.MultiWriter(child.Stderr, stderrLines)
		defer stdoutLines.flush()
		defer stderrLines.flush()
	}
	if call.mergeOutput {
		// 同じ書き先を渡すと、exec は1本のパイプで受けるので、書かれた順が保たれる。
		child.Stderr = child.Stdout
	}
	if call.input != "" {
		child.Stdin = strings.NewReader(call.input)
	}
	if err := child.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if call.mergeOutput {
			detail = strings.TrimSpace(stdout.String())
		}
		return "", &dockerFailure{output: detail, cause: err}
	}
	return stdout.String(), nil
}

// dockerFailure は、docker が失敗したことである。output は、失敗するまでに docker が
// 書いた出力（標準エラー。mergeOutput なら両方）で、エラーの文に含める。
//
// 出力にはシークレットが混じりうる（docker logs）。そのまま見せられない呼び出し側は、
// cause だけを書き、output は伏せてから使う。
type dockerFailure struct {
	output string
	cause  error
}

func (failure *dockerFailure) Error() string {
	if failure.output == "" {
		return failure.cause.Error()
	}
	return failure.output + ": " + failure.cause.Error()
}

func (failure *dockerFailure) Unwrap() error { return failure.cause }

// sayRun は、実行したコマンドと掛かった時間を、接続ログの debug3 に書く。失敗した
// ときは、docker の出力も書く。
func sayRun(ctx context.Context, call dockerCall, elapsed time.Duration, err error) {
	described := describeArguments(call.arguments)
	elapsed = connectionlog.Elapsed(elapsed)
	switch {
	case err == nil:
		connectionlog.Say(ctx, connectionlog.Full, "docker %s（%s）", described, elapsed)
	case stopCauseOf(ctx) != nil:
		// 経路の停止で打ち切ったコマンドは失敗ではない。出力も失敗として写さない。
		connectionlog.Say(ctx, connectionlog.Full, "docker %s を取り消しました（%s）。", described, elapsed)
	case call.absentSubject != "" && isAbsent(err):
		connectionlog.Say(ctx, connectionlog.Full, "docker %s（%s）：その%sはありません", described, elapsed,
			call.absentSubject)
	case call.callerShowsFailure:
		connectionlog.Say(ctx, connectionlog.Full, "docker %s は失敗しました（%s）。", described, elapsed)
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

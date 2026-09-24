package sshclient

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"sshc/internal/connectionlog"
	"sshc/internal/terminal"
)

// 接続の途中経過を、端末そのものへ書く。
//
// 接続失敗または切断の診断は `ssh -v` と同様に端末の stderr へ出力する。
//
// 既定は無言である。毎回この量が流れると、シェルの最初の一画面が
// 押し流されるため、必要な場合だけ verbosity を上げる。

// Verbosity は、どこまで言うかである。
type Verbosity int

const (
	// Quiet は、接続の途中を何も言わない。既定である。
	Quiet Verbosity = 0
	// Brief は `-v` に相当する。何が起き、どこへ繋いだかだけを言う。
	Brief Verbosity = 1
	// Detailed は `-vv` に相当する。試した鍵とその指紋、通った方式、
	// ホスト鍵の照合結果、経由地、端末と環境変数の要求、掛かった時間。
	Detailed Verbosity = 2
	// Full は `-vvv` に相当する。名乗った算法、agent の鍵の一覧、
	// keyboard-interactive の質問ごとの扱い、通った経路のアドレス。
	Full Verbosity = 3
)

// MaxVerbosity は、受け付ける上限である。設定の検証はここを見る。
const MaxVerbosity = int(Full)

// tracer は、設定された level 以下の診断だけを書き出す。ゼロ値は出力しない。
type tracer struct {
	level  Verbosity
	writer io.Writer
	// clock は掛かった時間を測る。テストが時計を止められるようにしてある。
	clock    func() time.Time
	progress func(terminal.ConnectionProgress)
}

// now は現在時刻を返す。nil の tracer でも呼び出せる。
func (t *tracer) now() time.Time {
	if t == nil || t.clock == nil {
		return time.Now()
	}
	return t.clock()
}

func newTracer(level Verbosity, writer io.Writer) *tracer {
	return &tracer{level: level, writer: writer, clock: time.Now}
}

// linePrefix は、接続ログの各行の頭に置く印である。
//
// 深さを行ごとに付けるのは、OpenSSH の debug1:／debug2:／debug3: と同じ
// 理由である。読む側は、その行がどの設定で出たかを行だけで判別でき、
// `debug2` で grep すれば深さ 2 の行だけを拾える。深さの印が無い `[sshc]` は、
// 設定に関係なく出る行（ProxyCommand の実行、再接続の通知）である。
func linePrefix(level Verbosity) string {
	return fmt.Sprintf("[sshc][debug%d] ", int(level))
}

// say は、その level が求められていれば 1 行書く。
//
// 端末は行末にCRLFを要る。生の\nだけを送ると、次の行が前の行の右端から
// 始まる。先頭にもCRLFを置くと、連続する診断の間が毎回空行になる。
func (t *tracer) say(level Verbosity, format string, args ...any) {
	if t == nil || t.writer == nil || level > t.level {
		return
	}
	_, _ = io.WriteString(t.writer, linePrefix(level)+fmt.Sprintf(format, args...)+"\r\n")
}

// announce は、verbosity に関係なく 1 行表示する（ProxyCommand の実行、VPN 経路の
// 時間のかかる段階など）。
func (t *tracer) announce(format string, args ...any) {
	if t == nil || t.writer == nil {
		return
	}
	_, _ = io.WriteString(t.writer, "[sshc] "+fmt.Sprintf(format, args...)+"\r\n")
}

// サーバーの banner をどこまで端末に出すか。
//
// banner は人が読むための文であり、一画面に収まらないものは案内ではない。
// 上限が無いと、サーバーが送る任意長の文をそのまま端末へ流すことになる。
const (
	maxBannerLines     = 40
	maxBannerLineRunes = 256
)

// banner は、認証の前にサーバーが送った文言を Brief から出す。
//
// OpenSSH は常に出すが、ここでは接続ログの一部として扱う。既定が無言なのは
// 接続の途中経過についてであり、それと別の扱いにすると「無言」の意味が二つになる。
// 文はサーバーが書いたものなので、行ごとに制御文字を落としてから出す。
func (t *tracer) banner(message string) {
	if !t.enabled(Brief) {
		return
	}
	lines := strings.Split(strings.TrimRight(message, "\r\n"), "\n")
	if len(lines) > maxBannerLines {
		lines = lines[:maxBannerLines]
	}
	t.say(Brief, "サーバーからの案内：")
	for _, line := range lines {
		t.say(Brief, "  %s", terminal.DisplayText(line, maxBannerLineRunes))
	}
}

// since は、始まりからの経過を返す。Full のときだけ意味を持つ。
func (t *tracer) since(start time.Time) time.Duration { return t.now().Sub(start) }

// enabled は、この level の診断が有効かを返す。
func (t *tracer) enabled(level Verbosity) bool {
	return t != nil && t.writer != nil && level <= t.level
}

// logWriter は、この tracer を connectionlog の書き先として見せる。VPN の経路の
// ように、この package の外で輸送を用意する部品が、同じ接続ログへ書くために使う。
type logWriter struct{ trace *tracer }

func (writer logWriter) Enabled(level connectionlog.Level) bool {
	return writer.trace.enabled(Verbosity(level))
}

func (writer logWriter) Write(level connectionlog.Level, message string) {
	if level == connectionlog.Notice {
		writer.trace.announce("%s", message)
		return
	}
	writer.trace.say(Verbosity(level), "%s", message)
}

// withLog は、この tracer を書き先に足した ctx を返す。
func (t *tracer) withLog(ctx context.Context) context.Context {
	return connectionlog.With(ctx, logWriter{trace: t})
}

func (t *tracer) stage(phase string, target Target, hop, hops int) {
	if t == nil || t.progress == nil {
		return
	}
	t.progress(terminal.ConnectionProgress{
		Phase: phase, Alias: target.Alias, HostName: target.HostName,
		User: target.User, Hop: hop, Hops: hops,
	})
}

// describeKey は、鍵を種類と SHA256 指紋で言う。
//
// `ssh -v` が鍵を言うときと同じ形である。指紋があれば、ユーザーは
// `ssh-keygen -lf` の出力や known_hosts の行と突き合わせられる。
func describeKey(key ssh.PublicKey) string {
	return key.Type() + " " + ssh.FingerprintSHA256(key)
}

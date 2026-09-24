package vpn

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"sshc/internal/connectionlog"
)

// 経路の先の接続先へ、接続1本ぶんの中継を開く。
//
// engine はコンテナの中の connect を docker exec -i で起動し、その標準入出力を
// 接続として使う。コンテナの中で作ったソケットをホストと共有する方法は、Docker
// Desktop（macOS、Windows）ではホストから開けないので使わない。

// ErrTargetFailed は、経路はあるが、接続先へ繋げなかったことを表す。
var ErrTargetFailed = errors.New("the vpn route could not reach the destination")

// TargetFailure は、接続先へ繋げなかったことと、その理由である。
//
// errors.Is(err, ErrTargetFailed) でも見分けられる。
type TargetFailure struct {
	Profile string
	// Destination は、接続先の表記（`host:port`）である。
	Destination string
	Reason      FailureReason
}

func (failure *TargetFailure) Error() string {
	return fmt.Sprintf("%v: %s via %s: %s", ErrTargetFailed, failure.Destination, failure.Profile, failure.Reason)
}

func (failure *TargetFailure) Unwrap() error { return ErrTargetFailed }

const (
	// connectPath は、コンテナの中の中継の場所である（container/Dockerfile）。
	connectPath = "/usr/local/lib/sshc-vpn/connect"
	// targetConnectTimeout は、接続先へ繋がるまで待つ上限である。connect の中の
	// socat が自分で諦める時間（connect.sh の connect_timeout_seconds）より長くし、
	// 理由を読めるようにする。
	targetConnectTimeout = 30 * time.Second

	// connectFailurePrefix は、connect が繋げなかった理由を書く行の始まりである。
	connectFailurePrefix = "sshc-vpn-failure: "
	// connectNotePrefix は、connect が行ったことを書く行の始まりである。
	connectNotePrefix = "sshc-vpn-note: "
	// maxConnectLines は、connect の標準エラーを接続ログへ写すために覚える行数の上限である。
	maxConnectLines = 40
	// connectStartedMark は、socat が接続先へ繋がり、中継を始めたときに書く文である。
	// イメージのパッケージは固定してあるので、socat の版とこの文も変わらない。
	connectStartedMark = "starting data transfer loop"
	// maxConnectLineBytes は、connect の標準エラーの1行を覚える上限である。
	maxConnectLineBytes = 4 << 10
)

// connectTarget は、動いている経路の中から、destination への接続を1本開く。
//
// 返すのは、接続先へ繋がったあとの接続である。繋げなかったときは、connect が
// 書いた理由を TargetFailure で返す。
func (manager *Manager) connectTarget(ctx context.Context, profileName string, destination Endpoint) (net.Conn, error) {
	watch := newConnectWatch()
	arguments := []string{"exec", "--interactive", manager.containerName(profileName),
		connectPath, destination.Host, strconv.Itoa(destination.Port)}
	connectionlog.Say(ctx, connectionlog.Full, "docker %s", describeArguments(arguments))
	conn, err := manager.docker.stream(watch, arguments...)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	timer := time.NewTimer(targetConnectTimeout)
	defer timer.Stop()
	// connect が書いた行を接続ログへ写す。繋がった場合も、繋げなかった場合も写す。
	defer watch.describe(ctx)
	refuse := func(reason FailureReason) error {
		_ = conn.Close()
		return &TargetFailure{Profile: profileName, Destination: destination.Address(), Reason: reason}
	}
	select {
	case <-watch.started:
		return conn, nil
	case <-conn.Exited():
		return nil, refuse(watch.failureReason())
	case <-timer.C:
		return nil, refuse(FailureTimeout)
	case <-ctx.Done():
		_ = conn.Close()
		return nil, ctx.Err()
	}
}

// connectWatch は、connect の標準エラーを1行ずつ読み、接続先へ繋がったか、
// 繋げなかった理由を覚える。
type connectWatch struct {
	// started は、接続先へ繋がって中継が始まると閉じる。
	started     chan struct{}
	startedOnce sync.Once

	mutex   sync.Mutex
	pending []byte
	failure FailureReason
	// notes は connect が行ったこと、lines はそれ以外の標準エラーの行（socat と
	// docker の出力）である。接続ログへ写す。
	notes []string
	lines []string
	// spoke は、connect が何か書いたことを表す。何も書かずに終わったなら、connect
	// まで届いていない（コンテナが無い、など）。
	spoke bool
}

func newConnectWatch() *connectWatch {
	return &connectWatch{started: make(chan struct{})}
}

func (watch *connectWatch) Write(chunk []byte) (int, error) {
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	watch.pending = append(watch.pending, chunk...)
	for {
		end := bytes.IndexByte(watch.pending, '\n')
		if end < 0 {
			break
		}
		watch.readLine(string(watch.pending[:end]))
		watch.pending = watch.pending[end+1:]
	}
	// 改行の無いまま長く続く出力は覚えない。
	if len(watch.pending) > maxConnectLineBytes {
		watch.pending = watch.pending[:0]
	}
	return len(chunk), nil
}

// readLine は、1行を読む。mutex を握って呼ぶこと。
func (watch *connectWatch) readLine(line string) {
	if note, found := strings.CutPrefix(strings.TrimSpace(line), connectNotePrefix); found {
		watch.spoke = true
		watch.notes = appendBounded(watch.notes, note)
		return
	}
	if strings.TrimSpace(line) != "" {
		watch.lines = appendBounded(watch.lines, withoutSocatTimestamp(strings.TrimSpace(line)))
	}
	if reason, found := strings.CutPrefix(strings.TrimSpace(line), connectFailurePrefix); found {
		watch.spoke = true
		if knownFailureReasons[FailureReason(reason)] {
			watch.failure = FailureReason(reason)
		}
		return
	}
	// socat の診断は「日付 socat[pid] 重大度 文」の形である。
	if strings.Contains(line, " socat[") {
		watch.spoke = true
	}
	if strings.Contains(line, connectStartedMark) {
		watch.startedOnce.Do(func() { close(watch.started) })
	}
}

// socatTimestamp は、socat が診断の行の頭に付ける時刻である。コンテナの時計は UTC
// なので、接続ログと記録の現地時刻と並ぶと食い違って見える。
var socatTimestamp = regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} `)

// withoutSocatTimestamp は、socat の行から時刻を外す。
func withoutSocatTimestamp(line string) string {
	return socatTimestamp.ReplaceAllString(line, "")
}

// failureReason は、connect が終わったあとで、繋げなかった理由を返す。
func (watch *connectWatch) failureReason() FailureReason {
	watch.mutex.Lock()
	defer watch.mutex.Unlock()
	switch {
	case watch.failure != "":
		return watch.failure
	case watch.spoke:
		// connect は接続先へ繋ぎに行き、socat が諦めた（拒否、無応答、経路なし）。
		return FailureTargetUnreachable
	default:
		return FailureTunnelLost
	}
}

// appendBounded は、上限の行数を超えないように行を足す。
func appendBounded(lines []string, line string) []string {
	if len(lines) >= maxConnectLines {
		return lines
	}
	return append(lines, line)
}

// describe は、connect が行ったことを debug2 に、socat と docker の出力を debug3 に
// 写す。繋げなかったときは、出力も debug2 に写す（原因はたいていそこにある）。
func (watch *connectWatch) describe(ctx context.Context) {
	watch.mutex.Lock()
	notes := append([]string(nil), watch.notes...)
	lines := append([]string(nil), watch.lines...)
	watch.mutex.Unlock()
	level := connectionlog.Full
	select {
	case <-watch.started:
	default:
		level = connectionlog.Detailed
	}
	for _, note := range notes {
		connectionlog.Say(ctx, connectionlog.Detailed, "コンテナの中継：%s", note)
	}
	for _, line := range lines {
		connectionlog.Say(ctx, level, "  %s", line)
	}
}

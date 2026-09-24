package vpn

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"sshc/internal/connectionlog"
)

// 経路ごとに、sshcエンジンが行ったことを記録する。
//
// 接続ログ（Terminal と CLI の [sshc][debugN]）へ書く行を、同じ深さの印を付けて
// ここにも残す。sshc vpn logs と画面の「ログ」は、コンテナのログと一緒にこれを
// 見せる。コンテナが起動する前に失敗した場合（イメージの作成、Docker が無い）、
// 残るのはこの記録だけである。CLI の接続は sshcエンジンの中で経路を用意するので、
// CLI の接続ログにはここから写す。

// maxRecordLines は、経路ひとつぶんの記録を覚える行数の上限である。古い行から
// 捨てる。数回ぶんの接続の試みが収まる長さにする。
const maxRecordLines = 400

// attemptRecord は、経路ひとつぶんの記録である。connectionlog の書き先になる。
type attemptRecord struct {
	mutex sync.Mutex
	lines []string
}

// Enabled は、どの深さの行も記録することを表す。
func (record *attemptRecord) Enabled(connectionlog.Level) bool { return true }

// Write は、1 行を時刻と深さの印を付けて記録する。
func (record *attemptRecord) Write(level connectionlog.Level, message string) {
	record.mutex.Lock()
	defer record.mutex.Unlock()
	record.lines = append(record.lines,
		fmt.Sprintf("%s %s %s", time.Now().Format("15:04:05"), levelMark(level), message))
	if overflow := len(record.lines) - maxRecordLines; overflow > 0 {
		record.lines = append([]string(nil), record.lines[overflow:]...)
	}
}

// levelMark は、記録の行に付ける深さの印である。接続ログと同じく、設定に関係なく
// 出す行は [sshc]、それ以外は [debugN] とする。
func levelMark(level connectionlog.Level) string {
	if level == connectionlog.Notice {
		return "[sshc]"
	}
	return fmt.Sprintf("[debug%d]", int(level))
}

// text は、記録を 1 つの文字列で返す。
func (record *attemptRecord) text() string {
	record.mutex.Lock()
	defer record.mutex.Unlock()
	return strings.Join(record.lines, "\n")
}

// recordingKey は、その ctx がすでにプロファイルの記録へ書いていることの印である。
type recordingKey struct{ profile string }

// recording は、このプロファイルの記録を書き先に足した ctx を返す。すでに足して
// あれば、そのまま返す（同じ行を二重に記録しない）。
func (manager *Manager) recording(ctx context.Context, profileName string) context.Context {
	if ctx.Value(recordingKey{profile: profileName}) != nil {
		return ctx
	}
	ctx = context.WithValue(ctx, recordingKey{profile: profileName}, true)
	return connectionlog.With(ctx, &manager.state(profileName).record)
}

// maxShownOutputLines は、失敗したコマンドの出力を接続ログへ写す行数の上限である。
// 原因の行はたいてい最後にある。
const maxShownOutputLines = 30

// sayOutput は、コマンドの出力の最後の行を、字下げして接続ログへ書く。
func sayOutput(ctx context.Context, level connectionlog.Level, output string) {
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	if len(lines) > maxShownOutputLines {
		lines = lines[len(lines)-maxShownOutputLines:]
	}
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			connectionlog.Say(ctx, level, "  %s", line)
		}
	}
}

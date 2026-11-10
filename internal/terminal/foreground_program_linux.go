package terminal

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// userHZ は、/proc の時間の単位である。Linux は USER_HZ の刻みで報告し、Go がビルドできるどのアーキテクチャでも
// 100 である。
const userHZ = 100

// ForegroundProgram は、シェルが前面で動かしているプログラムを /proc から読む。
// 名前はカーネルが覚えている実行ファイルの名前（comm、15 バイトまで）である。
func (p *unixProcess) ForegroundProgram() (ForegroundProgram, bool) {
	group, ok := p.foregroundCommandGroup()
	if !ok {
		return ForegroundProgram{}, false
	}
	// プロセスグループの番号は、その先頭のプロセスの番号でもある。先頭が先に
	// 終わったグループ（パイプラインの途中など）は読めず、分からないと答える。
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(group) + "/stat")
	if err != nil {
		return ForegroundProgram{}, false
	}
	uptime, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return ForegroundProgram{}, false
	}
	name, started, ok := parseProcessStat(string(stat))
	since, sinceOK := parseUptime(string(uptime))
	if !ok || !sinceOK || name == "" {
		return ForegroundProgram{}, false
	}
	return ForegroundProgram{Group: group, Name: name, Running: max(since-started, 0)}, true
}

// parseProcessStat は、/proc/<pid>/stat の行 `pid (name) state ppid ... starttime ...` から、名前と（起動から
// 数えた）開始時刻を読む。名前そのものが空白や括弧を含むことがあるので、名前は最後の `)` で終わる。
func parseProcessStat(stat string) (string, time.Duration, bool) {
	open, closing := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
	if open < 0 || closing < open {
		return "", 0, false
	}
	// 名前のあとには、3番目（state）以降の欄が続く。starttime は22番目である。
	const startTimeField = 22 - 3
	fields := strings.Fields(stat[closing+1:])
	if len(fields) <= startTimeField {
		return "", 0, false
	}
	ticks, err := strconv.ParseUint(fields[startTimeField], 10, 64)
	if err != nil {
		return "", 0, false
	}
	name := cleanDisplayText(stat[open+1:closing], MaxTitle)
	return name, time.Duration(ticks) * time.Second / userHZ, true
}

// parseUptime は、マシンが起動してからの長さを /proc/uptime から読む。
func parseUptime(uptime string) (time.Duration, bool) {
	fields := strings.Fields(uptime)
	if len(fields) == 0 {
		return 0, false
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(seconds * float64(time.Second)), true
}

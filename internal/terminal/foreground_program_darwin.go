package terminal

import (
	"time"

	"golang.org/x/sys/unix"
)

// ForegroundProgram は、シェルが前面で動かしているプログラムを sysctl（kern.proc.pid）で
// 読む。名前はカーネルが覚えている実行ファイルの名前（p_comm、16 バイトまで）である。
func (p *unixProcess) ForegroundProgram() (ForegroundProgram, bool) {
	group, ok := p.foregroundCommandGroup()
	if !ok {
		return ForegroundProgram{}, false
	}
	// プロセスグループの番号は、その先頭のプロセスの番号でもある。先頭が先に
	// 終わったグループ（パイプラインの途中など）は読めず、分からないと答える。
	info, err := unix.SysctlKinfoProc("kern.proc.pid", group)
	if err != nil || int(info.Proc.P_pid) != group {
		return ForegroundProgram{}, false
	}
	name := cleanDisplayText(unix.ByteSliceToString(info.Proc.P_comm[:]), MaxTitle)
	if name == "" {
		return ForegroundProgram{}, false
	}
	started := time.Unix(info.Proc.P_starttime.Unix())
	return ForegroundProgram{Group: group, Name: name, Running: max(time.Since(started), 0)}, true
}

package vt

// sshc がこのパッケージの写しに足したもの（README.sshc.md）。右端で折り返しを待つ（atPhantom）のをやめるところを、
// xterm.js にそろえる。xterm.js は、行を差し込む・消す IL・DL、逆の改行 RI、カーソルの復元（DECRC、SCORC、?1048l、
// ?1049l）を受けると、先にカーソルを右端のマスへ戻す（InputHandler の _restrictCursor）ので、次の字は次の行へ
// 折り返さない。上流はどれも待ちを残すので、次の字を次の行に書き、IL と RI のあとでは別の行の字を上書きする。
// ICH・DCH の待ちは shiftCells がやめる。

import "github.com/charmbracelet/x/ansi"

// stopWaitingToWrapAsXtermJSDoes は、IL・DL・RI・DECRC・SCORC・?1048l・?1049l を受けたとき、上流の処理の前に
// 折り返しの待ちをやめる。上流の処理には、そのまま回す。
func (e *SnapshotEmulator) stopWaitingToWrapAsXtermJSDoes() {
	stopWaiting := func() bool {
		e.atPhantom = false
		return false
	}
	// SCORC（CSI u）は上流が扱わない。カーソルは戻らず、待ちだけをやめる。
	for _, command := range []int{'L', 'M', 'u'} {
		e.RegisterCsiHandler(command, func(ansi.Params) bool { return stopWaiting() })
	}
	for _, command := range []int{'M', '8'} {
		e.RegisterEscHandler(command, stopWaiting)
	}
	e.registerCcHandler(ansi.RI, stopWaiting)
	e.RegisterCsiHandler(ansi.Command('?', 0, 'l'), func(params ansi.Params) bool {
		for _, param := range params {
			if restoresTheCursor(param.Param(-1)) {
				e.atPhantom = false
			}
		}
		return false
	})
}

// restoresTheCursor は、DEC のモード mode をリセットすると、保存したカーソルを戻すかを返す。?47l と ?1047l は
// 代替画面から戻るだけで、xterm.js も待ちを残す。
func restoresTheCursor(mode int) bool {
	return mode == ansi.ModeSaveCursor.Mode() || mode == ansi.ModeAltScreenSaveCursor.Mode()
}

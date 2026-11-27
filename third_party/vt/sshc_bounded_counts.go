package vt

// sshc がこのパッケージの写しに足したもの（README.sshc.md）。出力が渡す回数のとおりに繰り返す上流の処理を、
// 画面の幅までの回数で呼ぶ。

import "github.com/charmbracelet/x/ansi"

// boundCountsByTheScreen は、CHT（CSI Ps I）、CBT（CSI Ps Z）、ECH（CSI Ps X）の回数を、画面の幅までにして
// 上流の処理に渡す。上流は回数のとおりに繰り返すが、パーサーは回数に上限を設けないので、数バイトの出力で
// いくらでも長く止まる。エンジンはセッションのロックを持ったまま出力を読む。どれも、幅より多く繰り返しても
// 画面は変わらない。タブは1回に1つ以上進み、行の端で止まる。消すのは行の端までである。
func (e *SnapshotEmulator) boundCountsByTheScreen() {
	e.RegisterCsiHandler('I', func(params ansi.Params) bool {
		e.nextTab(e.countOnScreen(params))
		return true
	})
	e.RegisterCsiHandler('Z', func(params ansi.Params) bool {
		e.prevTab(e.countOnScreen(params))
		return true
	})
	e.RegisterCsiHandler('X', func(params ansi.Params) bool {
		e.eraseCharacter(e.countOnScreen(params))
		return true
	})
}

// countOnScreen は、回数の引数（省いたら1）を、画面の幅までにしたものである。
func (e *SnapshotEmulator) countOnScreen(params ansi.Params) int {
	count, _, _ := params.Param(0, 1)
	return min(count, e.Width())
}

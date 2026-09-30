package commandconn

// processTree は、起動したプログラムとその子孫をまとめて止める手段である。
//
// 直接の子だけを止めても、子が起こした孫（Windows の cmd.exe の下の proxy、
// aws ssm の下の session-manager-plugin）は残り、トンネルを開いたままにする。
// どう木をまとめるかは OS ごとに違うので、tree_unix.go と tree_windows.go が持つ。
type processTree interface {
	// kill は、木の全体を止める。
	kill() error
	// release は、木を扱うために持っている資源を手放す。Close の最後に一度だけ呼ぶ。
	release()
}

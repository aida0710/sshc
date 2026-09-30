package httpserver

// containsControlCharacter は、C0 制御文字（0x00-0x1f）か DEL（0x7f）を含むかを返す。
//
// PTY の行編集は、これらを文字ではなく操作として受け取る（Ctrl-U は行を消し、
// Ctrl-C は中断し、CR は実行する）。PTY へ書く文字列にこれが混ざると、書いた
// 文字列とは別のコマンドが組み立てられる。
func containsControlCharacter(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] == 0x7f {
			return true
		}
	}
	return false
}

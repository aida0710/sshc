package main

import (
	"strings"
	"testing"
)

// エコーされる回答の読み取り。
//
// ここを切り出してあるのは、/dev/tty を開く部分と違って試せるからである。端末を
// 開くテストは、それを走らせている人の端末を入力待ちで止めてしまう。
func TestReadEchoedLineTakesWhatWasTyped(t *testing.T) {
	cases := map[string]struct {
		input string
		want  string
	}{
		"改行で終わる":          {"yes\n", "yes"},
		"CRLF で終わる":       {"yes\r\n", "yes"},
		"改行なしで端末が閉じた":     {"yes", "yes"},
		"後ろに続きがある":        {"no\nignored\n", "no"},
		"fingerprint を打つ": {"SHA256:PJfawn3ikvz\n", "SHA256:PJfawn3ikvz"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := readEchoedLine(strings.NewReader(test.input))
			if err != nil {
				t.Fatalf("readEchoedLine(%q) error = %v", test.input, err)
			}
			if got != test.want {
				t.Errorf("readEchoedLine(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

// 何も打たれないまま端末が閉じたのは、回答ではない。
//
// 空文字を答えとして返せば、ssh には「yes ではない」と伝わって接続は止まる。止まる
// こと自体は安全側だが、なぜ止まったのかが誰にも分からない。読めなかったことは
// 読めなかったと言う。
func TestReadEchoedLineRefusesAnEmptyTerminal(t *testing.T) {
	if _, err := readEchoedLine(strings.NewReader("")); err == nil {
		t.Fatal("何も読めていないのにエラーが返らなかった")
	}
}

package acceptance_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sshc/internal/terminal"
)

// 画面に書いた秒数は、間隔から数えたものでなければならない。
//
// 設定画面は「5 回（最大 40 秒）」と書いている。その 40 は
// `internal/terminal` の backoff（1・2・5・10・15 秒）へ最大 jitter を
// 掛け、秒単位へ切り上げた値である。
//
// 和が二か所にあると、片方だけが古くなる。間隔を伸ばしたユーザーは Go 側を
// 直して緑を見て、画面はその日から誤りをつき始める。そして読むユーザーは、画面の方を
// 信じる。README の保管庫の寿命を定数から組み立てているのと同じ理由で、ここも
// 数えて突き合わせる。
func TestTheSettingsScreenSaysTheWindowTheGapsActuallyMakeUp(t *testing.T) {
	// 選べる回数と、その文言の鍵。
	choices := map[int]string{
		1: "terminal.reconnectOnce",
		2: "terminal.reconnectTwice",
		3: "terminal.reconnectThrice",
		5: "terminal.reconnectFive",
	}

	for _, language := range []string{"ja", "en"} {
		path := filepath.Join("..", "..", "web", "src", "i18n", "messages", language+".ts")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		messages := string(body)

		for attempts, key := range choices {
			line := messageFor(t, messages, key)
			seconds := fmt.Sprintf("%d", int(terminal.ReconnectWindow(attempts).Seconds()))
			if !strings.Contains(line, seconds) {
				t.Errorf("%s の %s が %s 秒を言っていない: %q\n"+
					"  間隔を変えたなら、この文言も変えること。", language, key, seconds, line)
			}
		}

		// 既定の選択肢も、既定の回数の窓を言う。
		fallback := messageFor(t, messages, "terminal.reconnectDefault")
		window := fmt.Sprintf("%d", int(terminal.ReconnectWindow(terminal.MaxReconnects).Seconds()))
		if !strings.Contains(fallback, window) {
			t.Errorf("%s の既定の選択肢が %s 秒を言っていない: %q", language, window, fallback)
		}
		if !strings.Contains(fallback, fmt.Sprintf("%d", terminal.MaxReconnects)) {
			t.Errorf("%s の既定の選択肢が %d 回を言っていない: %q", language, terminal.MaxReconnects, fallback)
		}
	}
}

// messageFor は、その鍵の行を返す。
func messageFor(t *testing.T, messages, key string) string {
	t.Helper()
	marker := `"` + key + `":`
	start := strings.Index(messages, marker)
	if start < 0 {
		t.Fatalf("文言 %s が無い", key)
	}
	rest := messages[start:]
	// 折り返された文言もあるので、次の鍵までを見る。
	if end := strings.Index(rest[len(marker):], `\n  "`); end >= 0 {
		return rest[:len(marker)+end]
	}
	if end := strings.Index(rest, "\n  \""); end >= 0 {
		return rest[:end]
	}
	return rest
}

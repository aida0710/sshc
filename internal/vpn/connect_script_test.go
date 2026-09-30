package vpn

import (
	"regexp"
	"strconv"
	"testing"
	"time"
)

// connectSeconds は、container/connect.sh に書いた待つ上限（秒）を返す。
func connectSeconds(t *testing.T, name string) int {
	t.Helper()
	contents, err := container.ReadFile("container/connect.sh")
	if err != nil {
		t.Fatal(err)
	}
	found := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `=([0-9]+)$`).FindSubmatch(contents)
	if found == nil {
		t.Fatalf("connect.sh に %s が無い", name)
	}
	seconds, err := strconv.Atoi(string(found[1]))
	if err != nil {
		t.Fatal(err)
	}
	return seconds
}

// connect は、鍵を待ち、名前解決し、接続先へ繋ぐまでを、engine が打ち切るより先に
// 諦めて理由を返す。engine の上限は docker exec の起動の前から数えるので、その余裕を
// 足しても先に諦める。engine が先に打ち切ると、利用者には「タイムアウト」しか残らない。
func TestConnectGivesUpBeforeTheEngineDoes(t *testing.T) {
	total := connectSeconds(t, "route_lock_seconds") + connectSeconds(t, "resolve_timeout_seconds") +
		connectSeconds(t, "connect_timeout_seconds")

	if waited := time.Duration(total)*time.Second + connectStartAllowance; waited >= targetConnectTimeout {
		t.Fatalf("connect が待つ合計と起動の余裕 %v が、engine が待つ上限 %v より短くない", waited, targetConnectTimeout)
	}
}

// 接続先の名前解決は、DNS サーバーが上限の台数まで並び、どれも応えなくても、connect が
// 名前解決を待つ上限の中で終わる。
func TestNameResolutionEndsWithinItsLimit(t *testing.T) {
	contents, err := container.ReadFile("container/agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	options := regexp.MustCompile(`options timeout:([0-9]+) attempts:([0-9]+)`).FindSubmatch(contents)
	if options == nil {
		t.Fatal("agent.sh の resolv.conf に options timeout と attempts が無い")
	}
	perServer, _ := strconv.Atoi(string(options[1]))
	attempts, _ := strconv.Atoi(string(options[2]))

	if worst := perServer * attempts * maxResolvers; worst > connectSeconds(t, "resolve_timeout_seconds") {
		t.Fatalf("DNS サーバーが %d 台とも応えないと %d 秒かかり、上限 %d 秒を超える",
			maxResolvers, worst, connectSeconds(t, "resolve_timeout_seconds"))
	}
}

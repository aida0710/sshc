package remotesync

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// syncRefusalsPath は、同期の画面が engine の code を文に訳す表である。
var syncRefusalsPath = filepath.Join("..", "..", "web", "src", "sync", "syncRefusals.ts")

// syncRefusalEntry は、表の 1 行（`  code: "message.key",`）の code を拾う。
var syncRefusalEntry = regexp.MustCompile(`(?m)^\s+([a-z0-9_]+):\s+"[^"]+",?$`)

// この package が返しうる code は、どれも同期の画面が自分の文に訳す。表に無い code は、
// 操作ごとの一般的な失敗の文と「Code: …」だけになり、何を直せばよいかが伝わらない
// （http:// のエンドポイントを入れると HTTPS が要ることを伝えられなかった）。
// code を足したら、同じ変更で表と ja.ts・en.ts に文を足す。
func TestEveryFailureCodeHasWordingInTheSyncScreen(t *testing.T) {
	body, err := os.ReadFile(syncRefusalsPath)
	if err != nil {
		t.Fatal(err)
	}
	translated := map[string]bool{}
	for _, match := range syncRefusalEntry.FindAllStringSubmatch(string(body), -1) {
		translated[match[1]] = true
	}
	// 拾い方を壊したときに、何も比べずに通らないようにする。
	if len(translated) < 10 {
		t.Fatalf("only %d codes were read from %s; the way they are collected is broken", len(translated), syncRefusalsPath)
	}

	codes := []string{internalFailure.Code}
	for _, rule := range failureRules {
		codes = append(codes, rule.failure.Code)
	}
	for _, refusal := range TargetRefusals {
		codes = append(codes, refusal.Error())
	}
	for _, code := range codes {
		if !translated[code] {
			t.Errorf("%s has no wording in %s", code, syncRefusalsPath)
		}
	}
}

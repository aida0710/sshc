package main

import (
	"context"
	"os/exec"
	"time"
)

// 入口をブラウザで開く。
//
// **開けなくても失敗ではない。** URL は必ず標準出力へ出しているので、開かな
// かった人の手元には貼れる綴りが残る。画面の無い機械では開く相手が居らず、
// それは壊れていることではない——`sshc engine` を走らせている端末で URL を
// 読むのが、その計算機での正しい姿である。
//
// **待たない。** ブラウザは前面に出るまで戻らないことがあり、そこで待つと
// コマンドが終わらない。起こしたら手を離す。

const browserTimeout = 5 * time.Second

// openInBrowser は、この OS の作法で URL を開く。開けたかどうかだけを返す。
func openInBrowser(ctx context.Context, url string) bool {
	name, args := browserCommand(url)
	if name == "" {
		return false
	}
	launchCtx, cancel := context.WithTimeout(ctx, browserTimeout)
	defer cancel()
	return exec.CommandContext(launchCtx, name, args...).Start() == nil
}

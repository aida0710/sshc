package main

import (
	"context"
	"os/exec"
	"time"
)

// アクセス URLをブラウザで開く。
//
// 開けなくても失敗ではない。資格情報を含むURLは標準出力へ残さず、画面の無い
// 機械では別の端末から`sshc`を実行して同じ入口を発行できる。
//
// 待たない。ブラウザは前面に出るまで戻らないことがあり、そこで待つと
// コマンドが終わらない。起動したら手を離す。

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

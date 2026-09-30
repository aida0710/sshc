package main

import "os/exec"

// アクセス URL をブラウザで開く。
//
// 開けなくても失敗にしない。`sshc` は URL を標準出力へ出してから開き、engine は
// 開けなければ `sshc` で開き直すよう案内する。画面の無いマシンでは、別の
// ターミナルから `sshc` を実行して URL を受け取れる。
//
// 待たない。ブラウザは前面に出るまで戻らないことがあり、そこで待つと
// コマンドが終わらない。起動したら手を離す。

// openInBrowser は、この OS の作法で URL を開く。起動できたかどうかだけを返す。
//
// 起動した子を context に結び付けない。exec.CommandContext は、Wait していない子も
// context が終わった時点で殺すので、戻った直後の xdg-open や open が URL を
// 渡す前に止まる。終わった子は裏で Wait し、engine のように長く動くプロセスに
// zombie を残さない。
func openInBrowser(url string) bool {
	name, args := browserCommand(url)
	if name == "" {
		return false
	}
	launcher := exec.Command(name, args...)
	configureBrowserLauncher(launcher)
	if err := launcher.Start(); err != nil {
		return false
	}
	go func() { _ = launcher.Wait() }()
	return true
}

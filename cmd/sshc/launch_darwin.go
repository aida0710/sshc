package main

import (
	"context"
	"os/exec"
	"time"
)

// bundleID は、束を LaunchServices が知っている名前である。
// **場所を覚えない。** どこに置かれていても、この名前で起こせる。
const bundleID = "com.github.aida0710.sshc"

// openPath は、束を起こす唯一のプログラムである。
//
// **絶対パスで指す。** 相対名は PATH を引く——起こすものが誰の PATH に何が
// 置かれているかで変わる。このアプリケーションが継ぎ目（`RunOutput`）で
// 相対パスを明示的に拒んでいるのと同じ理由である。
const openPath = "/usr/bin/open"

// launchTimeout は、束を起こす一回に上限を設ける。
//
// LaunchServices への依頼はすぐ返る。それでも上限を置くのは、返らなかった日に
// `sshc <接続先>` を打った人が待たされ続けないためである——待つ価値のあるもの
// は接続であって、これではない。
const launchTimeout = 5 * time.Second

// launchApp は、アプリを窓なしで起こす。
//
// **-g は前面に出さないという意味である。** 端末で打ったコマンドが、勝手に
// 画面を奪ってはならない。--hidden は窓を作らないという外殻への指示であり、
// メニューバーの項目は出るので、上がったことは見える。
func launchApp(ctx context.Context) bool {
	launchCtx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	return exec.CommandContext(launchCtx, openPath, "-g", "-b", bundleID, "--args", "--hidden").Run() == nil
}

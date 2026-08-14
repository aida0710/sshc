package main

import "os/exec"

// bundleID は、束を LaunchServices が知っている名前である。
// **場所を覚えない。** どこに置かれていても、この名前で起こせる。
const bundleID = "com.github.aida0710.sshc"

// launchApp は、アプリを窓なしで起こす。
//
// **-g は前面に出さないという意味である。** 端末で打ったコマンドが、勝手に
// 画面を奪ってはならない。--hidden は窓を作らないという外殻への指示であり、
// メニューバーの項目は出るので、上がったことは見える。
func launchApp() bool {
	return exec.Command("open", "-g", "-b", bundleID, "--args", "--hidden").Run() == nil
}

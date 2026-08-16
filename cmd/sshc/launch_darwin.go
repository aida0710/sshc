package main

import (
	"context"
	"errors"
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

// macDesktop は、LaunchServices を通して束を起こす。
type macDesktop struct{}

func newDesktopLauncher() desktopLauncher { return macDesktop{} }

// Available は、macOS では常に真である。
//
// **束が入っているかをここで調べない。** 調べる手段は LaunchServices に尋ねる
// ことで、それは起こすのと同じ問い合わせである。二度訊いて、その間に答えが
// 変わりうる隙を作るより、起こしてみて失敗を読む方が正しい。
func (macDesktop) Available() (bool, error) { return true, nil }

// Launch は、アプリを起こす。
//
// **-g は前面に出さないという意味である。** 端末で打ったコマンドが、勝手に
// 画面を奪ってはならない。ただし引数無しの `sshc` は、人が窓を見たくて打った
// ものなので、そちらは activateDesktop が前面に出す方を使う。
func (macDesktop) Launch(ctx context.Context) error {
	return runOpenBundle(ctx, "-b", bundleID)
}

// launchBackground は、接続経路が engine を必要としたときに束を窓なしで起こす。
//
// --hidden は窓を作らないという外殻への指示であり、メニューバーの項目は出るので、
// 上がったことは見える。
func launchBackground(ctx context.Context) bool {
	return runOpenBundle(ctx, "-g", "-b", bundleID, "--args", "--hidden") == nil
}

func runOpenBundle(ctx context.Context, args ...string) error {
	launchCtx, cancel := context.WithTimeout(ctx, launchTimeout)
	defer cancel()
	if err := exec.CommandContext(launchCtx, openPath, args...).Run(); err != nil {
		return errors.New("could not launch the sshc application; open it once from Applications")
	}
	return nil
}

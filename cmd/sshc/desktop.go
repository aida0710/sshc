package main

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"sshc/internal/handoff"
)

// desktopLauncher は、この計算機で画面付きの sshc を起こす方法である。
//
// **起こせるかどうかと、起こすことを分けてある。** 起こせない理由は環境ごとに
// 違い、言うべきことも違う——画面が無いのか、記録した実体が動いてしまったのか。
// bool ひとつではその区別が運べず、呼ぶ側は「起こせなかった」としか言えない。
type desktopLauncher interface {
	// Available は、いま起こせるかを答える。
	//
	// false と nil error は「この環境には画面付きの経路が無い」であり、
	// false と error は「経路はあるが壊れている」——後者だけが直し方を持つ。
	Available() (bool, error)
	Launch(context.Context) error
}

// runDesktop は、引数無しの `sshc` を引き受ける。
//
// **この入口は engine を起こさない。** 誰が engine の生存期間を持つかは外殻が
// 決めることで、端末で打たれた一語がそれを横取りしてはならない。ここがするのは
// 画面付きの外殻へ渡すことだけである。
func runDesktop(
	ctx context.Context, stateDir string, client *http.Client,
	launcher desktopLauncher, stderr io.Writer,
) int {
	// **headless が持っている engine を、画面付きで起こし直さない。** そちらは
	// 誰かが意図して端末で走らせているもので、後から上げた外殻は lock を取れず
	// 死ぬ。何が起きているかを言って、止め方を渡す方が短い。
	if status, err := engineStatus(ctx, stateDir, client); err == nil &&
		status.Owner == handoff.OwnerHeadless {
		fmt.Fprintln(stderr, "sshc: a headless engine owns this installation")
		fmt.Fprintln(stderr, "sshc: stop it first, then run sshc again")
		return 1
	}

	// handoff が古いままか、engine が居ないかは、ここでは区別しない。どちらでも
	// することは同じ——外殻を起こす。生きた desktop なら、外殻は二つ目の実体を
	// 作らず既存の窓を前へ出す。
	available, err := launcher.Available()
	if err != nil {
		return noWindow(stderr, err.Error())
	}
	if !available {
		return noWindow(stderr, "no graphical desktop is available here")
	}
	if err := launcher.Launch(ctx); err != nil {
		return noWindow(stderr, err.Error())
	}
	return 0
}

// noWindow は、窓を開けなかった理由と、窓なしで engine を持つ方法を出す。
//
// **理由がどれであっても headless を添える。** 直し方を持つ理由——記録が無い、
// 実体が動いた——を出すだけだと、そのアプリケーションを入れる気の無い人に
// 「入れろ」としか言っていないことになる。CLI だけを置いた機械や、SSH で
// 入った先には、窓を開ける以外の答えがあり、それはこの端末で engine を持つ
// ことである。**両方渡して、選ぶのは打った人に任せる。**
func noWindow(stderr io.Writer, reason string) int {
	fmt.Fprintf(stderr, "sshc: %s\n", reason)
	fmt.Fprintln(stderr, "sshc: run sshc headless to keep an engine in this terminal instead")
	return 1
}

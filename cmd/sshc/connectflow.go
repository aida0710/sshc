package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"sshc/internal/handoff"
)

// engineProbe は、生きている engine に尋ねる二つの問いである。
//
// **どちらも認証済みの往復であって、ファイルの読み取りではない。** handoff は
// 誰かが書き残した紙で、engine が生きている証拠にはならない。所有者が誰かを
// 決めるのは、答えを返したその engine だけである。
type engineProbe interface {
	Status(context.Context) (statusAnswer, error)
	Connection(context.Context, string) (connectAnswer, error)
}

// errInterrupted は、待っている人が Ctrl-C を押したことを表す。
//
// 失敗ではないので、失敗の終了コードを返さない。130 は「シグナルで終わった」を
// シェルへ伝える番号である。
var errInterrupted = errors.New("interrupted")

// errEngineChanged は、待っていた engine がもう同じものではないことを表す。
//
// **別の engine が解錠されても、それはこの待ち手が待っていたものではない。**
// secret も owner も protocol も、待ち始めたときの一台を指している。入れ替わった
// ものへ黙って接続すれば、利用者が知らない engine が接続材料を渡すことになる。
var errEngineChanged = errors.New("the running sshc changed while waiting; run the command again")

// unlockPoll は、解錠されたかを尋ね直す間隔である。
//
// **上限は置かない。** 解錠は人が行うもので、人が席を外す時間に上限は無い。
// 終わるのは、解錠されたか、Ctrl-C か、engine が入れ替わったか消えたときである。
const unlockPoll = 250 * time.Millisecond

// waitForDesktopUnlock は、同じ engine が解錠されるまで待つ。
//
// 解錠は Electron の窓でも、別の端末の `sshc vault unlock` でも起こる。どちらも
// 同じ engine を変えるので、待ち手はどちらが起きたかを知る必要が無い。
func waitForDesktopUnlock(
	ctx context.Context, initial handoff.Handoff, probe engineProbe, poll time.Duration,
) error {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		status, err := probe.Status(ctx)
		switch {
		case ctx.Err() != nil:
			return errInterrupted
		case err != nil:
			// engine が消えた。待ち続ければ、二度と来ない解錠を待つことになる。
			return errors.New("sshc stopped while waiting")
		case status.Owner != initial.Owner || status.ProtocolVersion != initial.ProtocolVersion:
			return errEngineChanged
		case status.Vault && status.Unlocked:
			return nil
		}
		select {
		case <-ctx.Done():
			return errInterrupted
		case <-ticker.C:
		}
	}
}

// httpProbe は、handoff に書かれた一台へ尋ねる。
//
// **secret を握り直さない。** 待っているあいだに handoff が書き換わっても、
// この probe が話す相手は待ち始めた一台のままである。入れ替わりは status の
// 答えとして現れ、待ち手はそれを見て降りる。
type httpProbe struct {
	found  handoff.Handoff
	client *http.Client
}

func (probe httpProbe) Status(ctx context.Context) (statusAnswer, error) {
	return requestStatus(ctx, probe.found, probe.client)
}

func (probe httpProbe) Connection(ctx context.Context, alias string) (connectAnswer, error) {
	return requestConnection(ctx, probe.found, alias, probe.client)
}

// reachUnlockedEngine は、設計 8.2 の六つの経路を一箇所で辿る。
//
// **engine に届かないことは、保存済み無しで繋いでよいという許可ではない。**
// 保存済みの答えを使わない接続がほしい人には `ssh <接続先>` があり、それは
// このアプリケーションが ~/.ssh/config に一切触れないから常に動く。黙って
// 退けば、鍵のパスフレーズを毎回訊かれるのが engine の不在のせいだと分から
// ないまま、利用者がそれを普通だと思ってしまう。
func reachUnlockedEngine(
	ctx context.Context, stateDir string, client *http.Client,
	launcher desktopLauncher, newProbe func(handoff.Handoff) engineProbe, stderr io.Writer,
) (engineProbe, error) {
	found, status, err := liveEngineStatus(ctx, stateDir, client, newProbe)
	if err != nil {
		// engine が居ない。画面付きの外殻があるなら起こして待つ。無いなら、
		// この端末で engine を持つ方法を渡して終わる。
		available, availableErr := launcher.Available()
		if availableErr != nil {
			return nil, availableErr
		}
		if !available {
			return nil, errors.New("sshc is not running; run sshc headless in another terminal, or use ssh to connect without it")
		}
		if err := launcher.Launch(ctx); err != nil {
			return nil, err
		}
		waitForHandoff(ctx, stateDir)
		found, status, err = liveEngineStatus(ctx, stateDir, client, newProbe)
		if err != nil {
			return nil, err
		}
	}

	probe := newProbe(found)
	if status.Vault && status.Unlocked {
		return probe, nil
	}

	// **保管庫が無いことを、解錠されていることとして扱わない。** 無ければ
	// 保存された答えは一つも無く、それは engine が黙って渡せる状態ではない。
	missing := !status.Vault
	if status.Owner != handoff.OwnerDesktop {
		if missing {
			return nil, errors.New("this installation has no vault; run sshc vault create")
		}
		// 見えない窓の前で待たせない。headless を動かしている人は端末に居る。
		return nil, errors.New("the sshc vault is locked; run sshc vault unlock")
	}

	if missing {
		fmt.Fprintln(stderr, "sshc: this installation has no vault yet")
		fmt.Fprintln(stderr, "sshc: create one in the sshc window, or run sshc vault create here")
	} else {
		fmt.Fprintln(stderr, "sshc: the sshc vault is locked")
		fmt.Fprintln(stderr, "sshc: unlock it in the sshc window, or run sshc vault unlock here")
	}
	// 窓を一度だけ前へ出す。poll のたびに起こせば、待っているあいだじゅう
	// 画面を奪い続けることになる。
	if err := launcher.Launch(ctx); err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
	}
	if err := waitForDesktopUnlock(ctx, found, probe, unlockPoll); err != nil {
		return nil, err
	}
	return probe, nil
}

// liveEngineStatus は、handoff を読み、その一台へ実際に尋ねる。
func liveEngineStatus(
	ctx context.Context, stateDir string, client *http.Client,
	newProbe func(handoff.Handoff) engineProbe,
) (handoff.Handoff, statusAnswer, error) {
	found, err := readHandoff(stateDir)
	if err != nil {
		return handoff.Handoff{}, statusAnswer{}, err
	}
	status, err := newProbe(found).Status(ctx)
	if err != nil {
		return handoff.Handoff{}, statusAnswer{}, err
	}
	// handoff に書かれた所有者ではなく、答えた engine の言う所有者を正本にする。
	found.Owner = status.Owner
	found.ProtocolVersion = status.ProtocolVersion
	return found, status, nil
}

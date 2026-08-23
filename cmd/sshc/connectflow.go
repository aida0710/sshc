package main

import (
	"context"
	"errors"
	"io"
	"net/http"

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

// httpProbe は、handoff に書かれた一台へ尋ねる。
//
// **secret を握り直さない。** handoff が書き換わっても、この probe が話す相手は
// 掴んだときの一台のままである。入れ替わりは status の答えとして現れる。
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

// reachUnlockedEngine は、走っている engine を掴み、開いていることを確かめる。
//
// **起こさない。** engine を生かしておくのは人であり、このコマンドではない
// ——`sshc engine` を tmux なり systemd なりの下で走らせるのが、この道具の形で
// ある。だからここでできるのは、居ないことと、その起こし方を言うことだけである。
//
// 黙って退かない。このアプリケーションが ~/.ssh/config に一切触れないので
// `ssh` は常に動くが、黙って退けば、鍵のパスフレーズを毎回訊かれるのが engine の
// 不在のせいだと分からないまま、利用者がそれを普通だと思ってしまう。
func reachUnlockedEngine(
	ctx context.Context, stateDir string, client *http.Client,
	newProbe func(handoff.Handoff) engineProbe, stderr io.Writer,
) (engineProbe, error) {
	found, status, err := liveEngineStatus(ctx, stateDir, client, newProbe)
	if err != nil {
		return nil, errors.New("sshc is not running; run sshc engine in another terminal, or use ssh to connect without it")
	}

	probe := newProbe(found)
	if status.Vault && status.Unlocked {
		return probe, nil
	}
	// **保管庫が無いことを、解錠されていることとして扱わない。** 無ければ
	// 保存された答えは一つも無く、それは engine が黙って渡せる状態ではない。
	if !status.Vault {
		return nil, errors.New("this installation has no vault; run sshc vault create")
	}
	return nil, errors.New("the sshc vault is locked; run sshc vault unlock")
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

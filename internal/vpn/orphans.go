package vpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
)

// 前回の engine が残したコンテナを見つけて止める。
//
// コンテナは、利用者（uid）と workspace の2つの札で自分のものと見分ける。uid だけ
// では、同じ利用者が HOME を変えて動かしている別の engine のコンテナまで止めて
// しまう。

// workspaceIdentityLength は、workspace の識別子に使うハッシュの桁数である。
// コンテナ名に入るので短くし、衝突しない程度には長くする。
const workspaceIdentityLength = 12

// workspaceIdentity は、中継を置く directory から workspace の識別子を作る。
// 同じ workspace で起動し直した engine は、同じ識別子になる。
func workspaceIdentity(directory string) string {
	digest := sha256.Sum256([]byte(directory))
	return hex.EncodeToString(digest[:])[:workspaceIdentityLength]
}

// containerName は、この利用者のこの workspace のこのプロファイルのコンテナ名である。
func (manager *Manager) containerName(profileName string) string {
	return "sshc-vpn-" + profileName + "-" + strconv.Itoa(manager.owner) + "-" + manager.workspace
}

// orphanGate は、回収が終わるまで経路の起動を待たせる。
//
// 何も予告されていなければ、待たずに通す。予告は engine の起動の途中で、HTTP を
// 受け付ける前に行う。
type orphanGate struct {
	mutex sync.Mutex
	done  chan struct{}
}

func newOrphanGate() *orphanGate {
	done := make(chan struct{})
	close(done)
	return &orphanGate{done: done}
}

// expect は、これから回収が行われることを予告する。
func (gate *orphanGate) expect() {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	gate.done = make(chan struct{})
}

// finish は、回収が終わったことを知らせる。
func (gate *orphanGate) finish() {
	gate.mutex.Lock()
	defer gate.mutex.Unlock()
	select {
	case <-gate.done:
	default:
		close(gate.done)
	}
}

func (gate *orphanGate) wait(ctx context.Context) error {
	gate.mutex.Lock()
	done := gate.done
	gate.mutex.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ExpectOrphanDiscard は、このあと DiscardOrphans が呼ばれることを予告する。
// それが終わるまで、経路の起動を待たせる。
func (manager *Manager) ExpectOrphanDiscard() { manager.orphans.expect() }

// DiscardOrphans は、前回のengineが残したコンテナを止める。
//
// 引き継がない。そのコンテナがどの設定で経路を張ったのかを確かめられない以上、
// 使い続けるより止める方が安全である。Docker が無くても、予告した待ちは解く。
func (manager *Manager) DiscardOrphans(ctx context.Context) error {
	defer manager.orphans.finish()
	if _, err := manager.command(ctx); err != nil {
		return err
	}
	output, err := manager.docker.output(ctx, "ps", "--all", "--quiet",
		"--filter", "label="+ownerLabel+"="+strconv.Itoa(manager.owner),
		"--filter", "label="+workspaceLabel+"="+manager.workspace)
	if err != nil {
		return err
	}
	// 1台ずつ止めると、停止を待つ時間が台数だけ積み重なり、そのあいだ経路の
	// 起動も待たされる。並べて止める。
	var stopping sync.WaitGroup
	for _, container := range strings.Fields(output) {
		stopping.Go(func() { manager.stopContainer(ctx, container) })
	}
	stopping.Wait()
	return nil
}

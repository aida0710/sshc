package vpn

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 経路の状態（どのコンテナが動いているか）を、docker から短い時間だけ覚えて使い回す。
//
// 経路の状態を読むには docker ps が要る。Docker Desktop の mac では1回に1秒近く
// かかることがある。VPN画面の読み直し、別のタブ、CLI の段階の知らせが同じ時間に
// 読みに来ても、docker は1回だけ呼び、その結果を分け合う。Docker が使えないことも
// 同じ長さだけ覚え、docker info を読み取りのたびには呼ばない。sshcエンジンが自分で
// 起動・停止した経路は、覚えた内容より新しいので、docker を待たずに反映する。

// routeReadingLifetime は、docker から読んだ経路の状態を使い回す長さである。
//
// VPN画面の読み直しの間隔（5秒）より短くし、読み直すたびに新しい状態を見せる。
// 同じ時間に来た読み取りは、1回の docker ps を分け合う。sshcエンジンの外で起きた
// 変化（トンネルが切れてコンテナが終わった）が画面に出るまでの遅れは、この長さと
// 画面の読み直しの間隔の和までになる。
const routeReadingLifetime = 3 * time.Second

// routeReadingTimeout は、経路の状態を読みに行く docker を待つ上限である。応えない
// docker を待ち続けて、読み取りが積み重ならないようにする。
const routeReadingTimeout = 30 * time.Second

// routeReading は、docker から一度に読んだ経路の状態である。
type routeReading struct {
	// containers は、この engine のコンテナがあるプロファイルと、そのコンテナが
	// 動いているかである。
	containers map[string]bool
	// err は、読めなかった理由である（Docker が無い、動いていない）。
	err error
	// since は、読み始めたときの変更の番号である。これより後に sshcエンジンが
	// 起動・停止した経路は、読んだ内容より新しい。
	since uint64
	// readAt は、読み終えた時刻である。使い回してよいかの判定に使う。
	readAt time.Time
	// epoch は、読み始めたときの routeReadings.epoch である。
	epoch uint64
	// done は、読み終えると閉じる。
	done chan struct{}
}

// routeReadings は、最後に読み終えた経路の状態と、いま読んでいる途中のものである。
type routeReadings struct {
	mutex   sync.Mutex
	latest  *routeReading
	pending *routeReading
	// epoch は、覚えた状態を捨てるたびに増える。捨てる前に読み始めたものは、読み
	// 終えても覚えない。
	epoch uint64
}

// readRoutes は、使い回せる状態があればそれを、無ければ docker から読んで返す。
// 読んでいる途中なら、その結果を待つ。
func (manager *Manager) readRoutes(ctx context.Context) (*routeReading, error) {
	reading, fresh := manager.startReadingRoutes()
	if fresh {
		return reading, nil
	}
	select {
	case <-reading.done:
		return reading, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// knownRoutes は、使い回せる状態があれば返す。無ければ docker から読み始めて、
// 待たずに false を返す。
func (manager *Manager) knownRoutes() (*routeReading, bool) {
	return manager.startReadingRoutes()
}

// startReadingRoutes は、使い回せる状態があれば true と一緒に返す。無ければ、読んで
// いる途中のものを返す（読んでいなければ読み始める）。
func (manager *Manager) startReadingRoutes() (*routeReading, bool) {
	readings := &manager.readings
	readings.mutex.Lock()
	defer readings.mutex.Unlock()
	if latest := readings.latest; latest != nil && manager.now().Sub(latest.readAt) < routeReadingLifetime {
		return latest, true
	}
	if readings.pending == nil {
		reading := &routeReading{since: manager.changes.Load(), epoch: readings.epoch, done: make(chan struct{})}
		readings.pending = reading
		go manager.readRoutesFromDocker(reading)
	}
	return readings.pending, false
}

// readRoutesFromDocker は、docker から経路の状態を読み、読み終えたものとして覚える。
//
// 読みに来た要求の ctx には結び付けない。最初に来た要求が取り消されても、同じ結果を
// 待っているほかの要求のために読み切る。
func (manager *Manager) readRoutesFromDocker(reading *routeReading) {
	ctx, cancel := context.WithTimeout(manager.lifetime, routeReadingTimeout)
	defer cancel()
	reading.containers, reading.err = manager.readContainers(ctx)
	reading.readAt = manager.now()

	readings := &manager.readings
	readings.mutex.Lock()
	if reading.epoch == readings.epoch {
		readings.latest = reading
	}
	if readings.pending == reading {
		readings.pending = nil
	}
	readings.mutex.Unlock()
	close(reading.done)
}

// forgetRoutes は、覚えている経路の状態と、いま読んでいる途中のものを捨てる。次の
// 読み取りは docker から読み直す。
func (manager *Manager) forgetRoutes() {
	readings := &manager.readings
	readings.mutex.Lock()
	defer readings.mutex.Unlock()
	readings.epoch++
	readings.latest, readings.pending = nil, nil
}

// listContainers は、この engine のコンテナがあるプロファイルと、そのコンテナが
// 動いているかを docker から読む。docker は1回だけ呼ぶ。
func (manager *Manager) listContainers(ctx context.Context) (map[string]bool, error) {
	if _, err := manager.commandForReading(ctx); err != nil {
		return nil, err
	}
	format := "{{.Label \"" + profileLabel + "\"}}\t{{.State}}"
	output, err := manager.docker.output(ctx, "ps", "--all", "--format", format,
		"--filter", "label="+ownerLabel+"="+strconv.Itoa(manager.owner),
		"--filter", "label="+workspaceLabel+"="+manager.workspace)
	if err != nil {
		return nil, err
	}
	containers := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 2 || validateProfileName(fields[0]) != nil {
			continue
		}
		containers[fields[0]] = fields[1] == "running"
	}
	return containers, nil
}

// noteChange は、sshcエンジンがこの経路を起動・停止したことを記録する。docker から
// 読んだ状態より新しいので、状態を組み立てるときにこちらを使う。
func (manager *Manager) noteChange(state *routeState) {
	state.noteChanged(manager.changes.Add(1))
}

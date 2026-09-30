package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"sshc/internal/filelock"
)

// errEngineRunning は、エンジンを起動する資格を既に別のプロセスが握っていることを言う。
//
// 呼び出し側（runEngineWithDependencies）は、これを受けると走っている engine を
// 置き換えるかを決める（replaceRunningEngine）。置き換えないなら exit 1 で終わる。
var errEngineRunning = errors.New("an sshc engine is already running")

// lockEngineStart は、状態ディレクトリの engine.lock を OS のロックで押さえる。
//
// 仕組みそのものは internal/filelock にある。ここに残っているのは、この
// コマンドが状態ディレクトリからロックのパスを組み立てることと、engine.lock を
// 握っているのは別の engine だと言い換えることだけである。filelock の失敗は
// 包んで返すので、後始末のエラーも捨てずに済む。
func lockEngineStart(stateDir string) (func() error, error) {
	release, err := filelock.TryAcquire(filepath.Join(stateDir, "engine.lock"))
	if errors.Is(err, filelock.ErrHeld) {
		return nil, fmt.Errorf("%w: %w", errEngineRunning, err)
	}
	return release, err
}

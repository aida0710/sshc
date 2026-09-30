package main

import (
	"path/filepath"

	"sshc/internal/enginelock"
)

// errEngineRunning は、エンジンを起動する資格を既に別のプロセスが握っていることを言う。
//
// 呼び出し側（runEngineWithDependencies）は、これを受けると走っている engine を
// 置き換えるかを決める（replaceRunningEngine）。置き換えないなら exit 1 で終わる。
//
// enginelock の同じ値をそのまま指定しているので、ロック側が理由を包んで返しても
// 呼び出し側の分岐はそのまま成立し、包まれた後始末エラーも捨てずに済む。
var errEngineRunning = enginelock.ErrRunning

// lockEngineStart は、状態ディレクトリの engine.lock を OS のロックで押さえる。
//
// 仕組みそのものは internal/enginelock にある。ここに残っているのは、この
// コマンドが状態ディレクトリからロックのパスを組み立てるという事実だけである。
func lockEngineStart(stateDir string) (func() error, error) {
	return enginelock.Acquire(filepath.Join(stateDir, "engine.lock"))
}

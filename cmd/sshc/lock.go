package main

import (
	"path/filepath"

	"sshc/internal/enginelock"
)

// errEngineRunning は、エンジンを起こす資格を既に別のプロセスが握っていることを言う。
//
// **これは失敗ではない。** 求めていた状態——エンジンが 1 台居る——が既に
// 成立しているという報せであり、呼び出し側はそちらの入口を出して終わる。
//
// enginelock の同じ値をそのまま名指しているので、ロック側が理由を包んで返しても
// 呼び出し側の分岐はそのまま成立し、包まれた後始末エラーも捨てずに済む。
var errEngineRunning = enginelock.ErrRunning

// lockEngineStart は、状態ディレクトリの engine.lock を OS のロックで押さえる。
//
// 仕組みそのものは internal/enginelock にある。ここに残っているのは、この
// コマンドが状態ディレクトリからロックのパスを組み立てるという事実だけである。
func lockEngineStart(stateDir string) (func() error, error) {
	return enginelock.Acquire(filepath.Join(stateDir, "engine.lock"))
}

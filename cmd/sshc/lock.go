package main

import "errors"

// errEngineRunning は、エンジンを起こす資格を既に別のプロセスが握っていることを言う。
//
// **これは失敗ではない。** 求めていた状態——エンジンが 1 台居る——が既に
// 成立しているという報せであり、呼び出し側はそちらの入口を出して終わる。
var errEngineRunning = errors.New("another engine is already running")

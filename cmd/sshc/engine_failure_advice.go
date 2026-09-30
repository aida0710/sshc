package main

import (
	"fmt"
	"io"
)

// engine と vault にまつわる失敗のうち、どのコマンドでも利用者が次にすることが
// 同じものの案内。文面をここだけに置き、各コマンドの switch では書かない。
// engine が動いていない場合の文は engineNotRunning が持つ。
const (
	vaultMissingAdvice       = "no vault exists; run sshc vault create"
	vaultLockedAdvice        = "the vault is locked; run sshc vault unlock"
	engineIncompatibleAdvice = "the CLI and running engine are incompatible; update whichever is older and restart it"
)

// engineFailureAdvice は、classifyCommandFailure が付けた種別のうち、どのコマンドでも
// 同じ案内になるものの文を返す。コマンドごとに案内が違う種別なら false を返す。
func engineFailureAdvice(kind string) (string, bool) {
	switch kind {
	case "engine_not_running":
		return engineNotRunning{}.Error(), true
	case "engine_incompatible", "engine_mismatch":
		return engineIncompatibleAdvice, true
	case "vault_missing":
		return vaultMissingAdvice, true
	case "vault_locked":
		return vaultLockedAdvice, true
	default:
		return "", false
	}
}

// writeEngineFailureAdvice は、failure が共通の種別なら案内を stderr へ書いて true を返す。
// false なら何も書かないので、呼び出し側がコマンド固有の文を書く。
func writeEngineFailureAdvice(stderr io.Writer, failure commandFailure) bool {
	advice, common := engineFailureAdvice(failure.Kind)
	if !common {
		return false
	}
	fmt.Fprintf(stderr, "sshc: %s\n", advice)
	return true
}

package main

import (
	"context"
	"net/http"

	"sshc/internal/app"
)

// engineStatusReader は、home の handoff が指す engine に状態を尋ねる。
type engineStatusReader func(ctx context.Context, home string) (statusAnswer, error)

// vaultStateUnknownAdvice は、起動した engine の Vault の状態を読めなかったときの案内である。
const vaultStateUnknownAdvice = "could not read the vault state; run sshc vault status"

// readEngineStatus は、home の handoff が指す engine を確かめてから状態を尋ねる。
func readEngineStatus(ctx context.Context, home string) (statusAnswer, error) {
	stateDir, err := app.StateDir(home)
	if err != nil {
		return statusAnswer{}, err
	}
	client := &http.Client{Timeout: connectTimeout}
	_, answer, err := verifiedStatus(ctx, stateDir, client)
	return answer, err
}

// vaultNextStep は、service が起動した engine の Vault について、利用者が次にすることを返す。
// パスワードなしの Vault は engine が起動時にロックを解除するので、解除済みなら空を返す。
func vaultNextStep(ctx context.Context, readStatus engineStatusReader, home string) string {
	answer, err := readStatus(ctx, home)
	switch {
	case err != nil:
		return vaultStateUnknownAdvice
	case !answer.Vault:
		return vaultMissingAdvice
	case !answer.Unlocked:
		return vaultLockedAdvice
	default:
		return ""
	}
}

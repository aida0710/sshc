package main

import (
	"io"
	"strings"
	"testing"
)

// engine を使うコマンドは、engine や vault が使えないとき、どれも同じ文で次にする
// ことを案内する。
func TestEngineCommandsGiveTheSameAdviceWhenTheEngineOrVaultIsUnavailable(t *testing.T) {
	_, notRunning := readHandoff(t.TempDir())
	if notRunning == nil {
		t.Fatal("a state directory with no handoff was read as a running engine")
	}
	failures := []struct {
		name   string
		err    error
		advice string
	}{
		{name: "no engine", err: notRunning, advice: engineNotRunning{}.Error()},
		{name: "no vault", err: errEngineVaultMissing, advice: vaultMissingAdvice},
		{name: "locked vault", err: errEngineVaultLocked, advice: vaultLockedAdvice},
		{name: "another engine", err: errEngineIdentityMismatch, advice: engineIncompatibleAdvice},
	}
	commands := map[string]func(err error, stderr io.Writer) int{
		"otp":      func(err error, stderr io.Writer) int { return finishOTPFailure(false, err, io.Discard, stderr) },
		"terminal": func(err error, stderr io.Writer) int { return finishTerminalFailure(false, err, io.Discard, stderr) },
		"sftp":     func(err error, stderr io.Writer) int { return finishSFTPFailure(false, err, io.Discard, stderr) },
		"sync":     func(err error, stderr io.Writer) int { return finishSyncFailure(false, err, io.Discard, stderr) },
	}
	for commandName, finish := range commands {
		for _, failure := range failures {
			t.Run(commandName+"/"+failure.name, func(t *testing.T) {
				var stderr strings.Builder
				code := finish(failure.err, &stderr)
				if want := "sshc: " + failure.advice + "\n"; code != 1 || stderr.String() != want {
					t.Fatalf("exit = %d, stderr = %q; want 1 and %q", code, stderr.String(), want)
				}
			})
		}
	}
}

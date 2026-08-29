package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"sshc/internal/app"
	"sshc/internal/handoff"
	"sshc/internal/sshclient"
	"sshc/internal/validate"
)

// runRemote は非対話コマンドを実行し、stdout、stderr、終了コードを保持する。
func runRemote(
	ctx context.Context, alias, command, home, stateDir string, client *http.Client, stdin io.Reader, stdout, stderr io.Writer,
) int {
	if err := validate.Alias(alias); err != nil {
		fmt.Fprintf(stderr, "sshc: %q is not an alias this will connect to\n", alias)
		return 2
	}

	// 非対話実行では Vault の解錠を待機せず、施錠状態をエラーとして返す。
	session, err := reachUnlockedEngine(ctx, stateDir, client,
		func(found handoff.Handoff) engineProbe {
			return httpProbe{found: found, client: client}
		}, stderr)
	if err != nil {
		if errors.Is(err, errInterrupted) {
			return 130
		}
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return sshclient.RemoteFailureExit
	}

	answer, err := session.Connection(ctx, alias)
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return sshclient.RemoteFailureExit
	}
	writeConnectionNotices(stderr, answer)

	connection, err := app.NewCLIConnection(home, savedPassphraseFor(answer), savedPasswordFor(answer))
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return sshclient.RemoteFailureExit
	}
	code, err := connection.Run(ctx, alias, command, sshclient.Streams{
		In: stdin, Out: stdout, Err: stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", runAdvice(err, alias))
		return code
	}
	return code
}

// runAdvice は非対話実行エラーに復旧手順を追加する。
func runAdvice(err error, alias string) error {
	switch {
	case errors.Is(err, sshclient.ErrHostKeyUnknown):
		return fmt.Errorf("%w; connect once with sshc ssh %s to decide about its host key", err, alias)
	case errors.Is(err, sshclient.ErrPromptUnavailable):
		return fmt.Errorf("%w; confirm a saved credential for %s in Connections, or use sshc ssh %s without --non-interactive to answer the prompt", err, alias, alias)
	case errors.Is(err, sshclient.ErrProxyCommandWithJump):
		return fmt.Errorf("%w; keep whichever one you meant", err)
	}
	return err
}

// remoteCommand は引数をリモートシェル用の単一文字列に結合する。
// 引用規則はリモートシェルに依存するため、ローカルでは再引用しない。
func remoteCommand(words []string) string {
	return strings.Join(words, " ")
}

// savedPassphraseFor と savedPasswordFor は engine 応答を sshclient の検索関数へ変換する。
func savedPassphraseFor(answer connectAnswer) func(string) (string, bool) {
	if len(answer.Passphrases) == 0 {
		return nil
	}
	return func(relativePath string) (string, bool) {
		stored, found := answer.Passphrases[relativePath]
		return stored, found && stored != ""
	}
}

func savedPasswordFor(answer connectAnswer) func(sshclient.Target) (string, bool) {
	if len(answer.Passwords) == 0 {
		return nil
	}
	return func(candidate sshclient.Target) (string, bool) {
		stored, found := answer.Passwords[candidate.Alias]
		binding := answer.PasswordBindings[candidate.Alias]
		if !found || stored == "" || binding != candidate.AuthenticationBinding() {
			return "", false
		}
		return stored, true
	}
}

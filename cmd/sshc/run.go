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

// runRemote は、接続先でコマンドをひとつ走らせ、その終了状態をそのまま返す。
//
// **`sshc <接続先>` とは形が違う。** あちらは端末を開いて人が座るためのもので、
// 出力には画面制御が混ざる。こちらは集めて読むためのもので、stdout と stderr を
// 分け、終了状態を返す。保管庫が持っている鍵のパスフレーズを使えるのは同じで、
// **それがこの入口の理由である**——保存済みの答えを持っているのに、無人の
// 操作でそれを使えないのでは、保管庫が半分しか働いていない。
func runRemote(
	ctx context.Context, alias, command, home, stateDir string, client *http.Client,
	launcher desktopLauncher, stdin io.Reader, stdout, stderr io.Writer,
) int {
	if err := validate.Alias(alias); err != nil {
		fmt.Fprintf(stderr, "sshc: %q is not an alias this will connect to\n", alias)
		return 2
	}

	// **待たない。** `sshc <接続先>` は施錠された desktop を無期限に待つが、
	// それは人が窓を開けて解錠するのを待てる場面での話である。この入口は
	// 書かれた手順の中で走るものなので、答える人の居ない待ちは、ただ止まった
	// ままになる。施錠されているなら、そう言って降りる。
	session, err := reachUnlockedEngine(ctx, stateDir, client, launcher,
		func(found handoff.Handoff) engineProbe {
			return httpProbe{found: found, client: client}
		}, stderr, false)
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
	for _, warning := range answer.Warnings {
		fmt.Fprintf(stderr, "sshc: %s\n", warning)
	}

	connection, err := app.NewCLIConnection(home, savedPassphraseFor(answer), savedPasswordFor(answer))
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", err)
		return sshclient.RemoteFailureExit
	}
	code, err := connection.Run(ctx, alias, command, sshclient.Streams{
		In: stdin, Out: stdout, Err: stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sshc: %v\n", runAdvice(err))
		return code
	}
	return code
}

// runAdvice は、無人の実行が断った理由に「では何をすればよいか」を添える。
func runAdvice(err error) error {
	switch {
	case errors.Is(err, sshclient.ErrHostKeyUnknown):
		return fmt.Errorf("%w; connect once with sshc %s to decide about its host key", err, "<alias>")
	case errors.Is(err, sshclient.ErrProxyCommand):
		return fmt.Errorf("%w; ~/.ssh/config is untouched, so ssh %s still works", err, "<alias>")
	}
	return err
}

// remoteCommand は、打たれた語をひとつの文字列にする。
//
// **相手のシェルが解釈する一本の文字列である。** OpenSSH の `ssh host cmd args`
// と同じで、区切りは空白ひとつ、引用の規則は相手のシェルのものになる。こちらで
// 引用を付け直さないのは、相手が何のシェルかを知らないからである——PowerShell と
// bash では正解が違い、推測して包めば、どちらかで壊れる。
func remoteCommand(words []string) string {
	return strings.Join(words, " ")
}

// savedPassphraseFor と savedPasswordFor は、engine が返した答えを
// sshclient が引ける形にする。`runConnect` と同じ組み立てである。
func savedPassphraseFor(answer connectAnswer) func(string) (string, bool) {
	if len(answer.Passphrases) == 0 {
		return nil
	}
	return func(relativePath string) (string, bool) {
		stored, found := answer.Passphrases[relativePath]
		return stored, found && stored != ""
	}
}

func savedPasswordFor(answer connectAnswer) func(string) (string, bool) {
	if len(answer.Passwords) == 0 {
		return nil
	}
	return func(candidate string) (string, bool) {
		stored, found := answer.Passwords[candidate]
		return stored, found && stored != ""
	}
}

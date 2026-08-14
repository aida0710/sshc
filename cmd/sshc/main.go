package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sshc/internal/app"
	"sshc/internal/selfupdate"
	"sshc/internal/ui"
)

var version = "dev"

// announceEntrance はここで宣言される。フラグを解析するのはサブコマンドを見分けた
// 後だが、usage はどの経路からでもフラグを一覧できなければならないからである。
//
// **ブラウザは開かない。** 画面はデスクトップの外殻が出すので、このプロセスが
// 既定のブラウザを起こす経路は無くなった。書き出す先は、`sshc` を打った人の
// 端末である。
//
// 既定で書き出すのは、端末から打った人がそこを読むからである。**背後で上がる
// エージェントは -open=false を渡す**——あの 1 行は有効な bootstrap トークンを
// 運ぶので、journal やログファイルの置き場所として不適切である。フラグ名を
// 変えないのは、既に書かれている launchd と systemd の unit を壊さないためで
// ある。
var openBrowser = flag.Bool("open", true,
	"print the way into the UI on standard output; -open=false prints nothing")

// HelpSubcommand は使い方を出す語。
//
// これを予約するのは `open` と同じ理由である。裸の語は alias なので、これがないと
// "help" はホスト名として ssh へ渡り、使い方を求めた人は
// "Could not resolve hostname help" を受け取る。-h は flag パッケージが拾うが、
// 打つ言葉として自然なのはこちらだ。
const HelpSubcommand = "help"

// helpInvocation は、このプロセスが使い方を求めて起動されたかを報告する。
//
// 引数を伴う場合は使い方ではない。`sshc help me` は、まだ理解されていない何かで
// あって、"help" というホストへの接続ではない。
func helpInvocation(argv []string) bool {
	if len(argv) != 2 {
		return false
	}
	switch argv[1] {
	case HelpSubcommand, "-h", "--help":
		return true
	}
	return false
}

// askpassInvocation は、このプロセスが OpenSSH のプロンプトに答えるために起動された
// かを報告し、ヘルパーが読むべき引数を返す。
//
// サブコマンドの語は手で実行するための手段である。OpenSSH の実行方法ではない。
// SSH_ASKPASS はプログラムを指定し、OpenSSH はプロンプトだけを引数としてそのプログラム
// を exec する — シェルがないので、サブコマンドの語が入る場所はどこにもない。これが
// なかったために、出荷された機能はアプリケーション全体の二つ目のコピーをブラウザごと
// 起動し、ssh には決して送られてこないパスワードが渡された。見つけたのは、実物の sshd
// に対する統合テストスイートである。
//
// 目印にトークンを使うのは、それがちょうどひとつの接続のために存在し、この
// アプリケーション以外に設定するものがないからだ。エンドポイントも併せて必須なのは、

// usage は、このバイナリが答える語をすべて並べる。
//
// `sshc <alias>` は裸の語であり、ここに並ぶ語だけがそれより先に読まれる。
// それを書いておかないと、`open` という名前のホストに接続できない理由が
// どこにも書かれていないことになる。
func usage(out io.Writer) {
	fmt.Fprint(out, `usage:
  sshc                 open the user interface in the default browser
  sshc <alias>         connect to a host from ~/.ssh/config in this terminal
  sshc connect [text]  choose a host in this terminal, then connect
  sshc list            print every concrete Host alias, one per line
  sshc open            print a new way into the UI
  sshc status          print the engine's status as JSON, for the shell
  sshc help            print this

flags:
`)
	flag.PrintDefaults()
	fmt.Fprint(out, `
Those words are read before the alias, so a host named after one of them is
still reachable with ssh itself, but not through this command.
`)
}

func main() {
	if arguments, ok := openInvocation(os.Args); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			os.Exit(1)
		}
		if len(arguments) != 0 {
			fmt.Fprintln(os.Stderr, "sshc: open takes nothing")
			os.Exit(2)
		}
		os.Exit(runOpen(
			context.Background(), app.HandoffDir(home),
			&http.Client{Timeout: connectTimeout}, os.Stdout, os.Stderr,
		))
	}
	if helpInvocation(os.Args) {
		usage(os.Stdout)
		os.Exit(0)
	}
	if len(os.Args) == 2 && os.Args[1] == ListSubcommand {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runList(home, os.Stdout, os.Stderr))
	}
	// **外殻が読む口である。** handoff の秘密を持つのは Go 側だけなので、
	// メニューバーは自分では叩けず、この語を経由する。
	if len(os.Args) == 2 && os.Args[1] == StatusSubcommand {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runStatus(
			context.Background(), app.HandoffDir(home),
			&http.Client{Timeout: connectTimeout}, os.Stdout, os.Stderr,
		))
	}
	if query, ok := tuiInvocation(os.Args); ok {
		if len(os.Args) > 3 {
			fmt.Fprintln(os.Stderr, "usage: sshc connect [search]")
			os.Exit(2)
		}
		// 検索語はフラグではないが、ヘルプを求めた人に接続先を探させはしない。
		if query == "-h" || query == "--help" {
			usage(os.Stdout)
			os.Exit(0)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			os.Exit(1)
		}
		alias, err := chooseTUIHost(home, query, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			if errors.Is(err, errTUIClosed) {
				os.Exit(0)
			}
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runConnect(
			context.Background(), alias, home, app.HandoffDir(home),
			&http.Client{Timeout: connectTimeout}, os.Stdin, os.Stdout, os.Stderr,
		))
	}

	// `sshc <alias>` は接続する。askpass の分岐のあと、フラグ解析の前で判定する。alias は
	// 裸の語であり、flag.Parse はそこで止まってしまうため、接続する代わりにアプリケーション
	// が起動してしまうからだ。
	if alias, ok := connectInvocation(os.Args); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "sshc: %v\n", err)
			os.Exit(1)
		}
		os.Exit(runConnect(
			context.Background(), alias, home, app.HandoffDir(home),
			&http.Client{Timeout: connectTimeout}, os.Stdin, os.Stdout, os.Stderr,
		))
	}

	// 既定の usage はフラグしか知らない。サブコマンドは argv の裸の語であり、
	// flag パッケージから見えないので、ここで一度だけ書く。
	flag.Usage = func() { usage(flag.CommandLine.Output()) }
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// **アプリが消えたらエンジンも消える。** 親を見張るのは、通常の終了
	// 経路（親が kill する）が働かなかったときのためである。
	go watchParent(ctx, os.Getppid, parentTick, stop)

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	assets, err := ui.FS()
	if err != nil {
		logger.Error("load embedded UI", "error", err)
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		logger.Error("resolve home directory", "error", err)
		os.Exit(1)
	}

	parts := newPlatformParts()

	var announce func(string) error
	if *openBrowser {
		announce = func(entrance string) error {
			_, err := fmt.Fprintln(os.Stdout, entrance)
			return err
		}
	}

	dependencies := app.Dependencies{
		Random:   rand.Reader,
		Announce: announce,
		// このアプリケーションが自分自身以外のホストに接触する唯一の場所であり、
		// 誰かが求めたときにだけ行う。何も取得せず、何も置き換えない。
		// 新しいバージョンが公開されているかを報告するだけである。
		Updates: &selfupdate.Checker{
			API:  "https://api.github.com/repos/aida0710/sshc/releases/latest",
			HTTP: &http.Client{Timeout: 30 * time.Second},
		},
		Listen:    net.Listen,
		UI:        assets,
		Logger:    logger,
		Home:      home,
		Toolchain: parts.Toolchain,
		KeyAgent:  parts.KeyAgent,
		Lookup:    os.LookupEnv,
		Environ:   os.Environ,
	}
	if err := app.Run(ctx, dependencies, version); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("sshc stopped", "error", err)
		os.Exit(1)
	}
}

package httpserver

import (
	"errors"
	"log/slog"

	"sshc/internal/secret"
	"sshc/internal/snippets"
)

// StartupSnippet は、接続のたびに起動スニペットについて決めたことである。
//
// Command があれば、認証とリモートシェルの起動が済んでから送る。Notice があれば、
// 同じ時点でターミナルの接続ログへ 1 行書く。どちらも空なら何もしない。
type StartupSnippet struct {
	// Command は、変数（シークレットを含む）を展開したコマンドである。
	Command string
	// Notice は、送らなかったことを利用者に知らせる 1 行である。割り当て直すなど、
	// 利用者の操作で直せる理由のときだけ入れる。
	Notice string
}

// startupDestinationChangedNotice は、割り当てた時から接続先（VPNプロファイルを含む）が
// 変わったので起動スニペットを送らなかったことを知らせる。割り当てたホストは Snippets
// 画面でしか分からないので、直す場所もここで示す。
const startupDestinationChangedNotice = "割り当てた後で接続先、認証の設定、またはVPNプロファイルが変わったため、起動スニペットを送りませんでした。［Snippets］画面で接続先を確認して、もう一度割り当ててください。"

// prepareStartupSnippet は、alias に割り当てた起動スニペットをこの接続で送るかを決める。
//
// 照合は、snippets.Service が接続の直後に alias を解決し直した接続先で行い、接続が
// 使った Target とは照合しない。両者がずれるのは、同じ接続を開くあいだのごく短い時間に
// 設定が割り当て時の接続先へ戻った場合だけなので、セッションに binding を持たせて
// ここまで運ぶ配線は足さない。
func prepareStartupSnippet(service *snippets.Service, logger *slog.Logger, alias string) StartupSnippet {
	if service == nil {
		return StartupSnippet{}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	prepared, err := service.PrepareStartupCommand(alias)
	switch {
	case err == nil:
		return StartupSnippet{Command: prepared.Command}
	case errors.Is(err, snippets.ErrNoStartup):
		return StartupSnippet{}
	case errors.Is(err, snippets.ErrStartupDestinationChanged):
		logger.Warn("startup snippet was not sent", "alias", alias, "reason", err.Error())
		return StartupSnippet{Notice: startupDestinationChangedNotice}
	case errors.Is(err, secret.ErrLocked):
		// ロック中はスニペットの文書を読めず、割り当てがあるかも分からない。割り当てて
		// いないホストの接続でも毎回ここへ来るので、送らなかったとは記録しない。
		logger.Debug("startup snippet was not checked", "alias", alias, "reason", err.Error())
		return StartupSnippet{}
	default:
		logger.Warn("startup snippet was not sent", "alias", alias, "reason", err.Error())
		return StartupSnippet{}
	}
}

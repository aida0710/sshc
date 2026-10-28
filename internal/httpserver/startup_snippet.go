package httpserver

import (
	"errors"
	"log/slog"

	"sshc/internal/secret"
	"sshc/internal/snippets"
	"sshc/internal/terminalopen"
)

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
func prepareStartupSnippet(service *snippets.Service, logger *slog.Logger, alias string) terminalopen.StartupSnippet {
	if service == nil {
		return terminalopen.StartupSnippet{}
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	prepared, err := service.PrepareStartupCommand(alias)
	switch {
	case err == nil:
		return terminalopen.StartupSnippet{Command: prepared.Command}
	case errors.Is(err, snippets.ErrNoStartup):
		return terminalopen.StartupSnippet{}
	case errors.Is(err, snippets.ErrStartupDestinationChanged):
		logger.Warn("startup snippet was not sent", "alias", alias, "reason", err.Error())
		return terminalopen.StartupSnippet{Notice: startupDestinationChangedNotice}
	case errors.Is(err, secret.ErrLocked):
		// ロック中はスニペットの文書を読めず、割り当てがあるかも分からない。割り当てて
		// いないホストの接続でも毎回ここへ来るので、送らなかったとは記録しない。
		logger.Debug("startup snippet was not checked", "alias", alias, "reason", err.Error())
		return terminalopen.StartupSnippet{}
	default:
		logger.Warn("startup snippet was not sent", "alias", alias, "reason", err.Error())
		return terminalopen.StartupSnippet{}
	}
}

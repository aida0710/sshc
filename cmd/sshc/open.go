package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"sshc/internal/httpserver"
)

// runOpen は engine から一度限りの UI URL を取得して出力する。
// open が true の場合だけ URL をブラウザへ渡す。
func runOpen(ctx context.Context, environment commandEnvironment, open bool) int {
	return openUIURL(ctx, environment, uiURLRequest{
		call:    handoffCall{method: http.MethodPost, path: httpserver.OpenPath, body: strings.NewReader("{}")},
		client:  environment.client,
		explain: func(refusal engineProblem) string { return refusedRequestError(refusal).Error() },
	}, open)
}

// uiURLRequest は、engine に UI URL を 1 件発行させる要求である。
type uiURLRequest struct {
	call   handoffCall
	client *http.Client
	// explain は、engine が断った理由を、表示する文にする。
	explain func(engineProblem) string
}

// openUIURL は、必要なら engine をバックグラウンドで起動し、UI URL を 1 件発行させて
// 出力する。open が true の場合だけ URL をブラウザへ渡す。
func openUIURL(ctx context.Context, environment commandEnvironment, request uiURLRequest, open bool) int {
	stdout, stderr := environment.stdout, environment.stderr
	found, err := ensureBackgroundEngine(ctx, environment, startBackgroundEngine)
	if err != nil {
		return reportEngineUnreachable(ctx, err, stderr)
	}
	var answer struct {
		URL string `json:"url"`
	}
	err = newHandoffEndpoint(found, request.client).exchange(ctx,
		request.call, handoffAnswer{status: http.StatusOK, into: &answer})
	if ctx.Err() != nil {
		return reportEngineUnreachable(ctx, ctx.Err(), stderr)
	}
	refusal, refused := engineRefusal(err)
	switch {
	case isTransportProblem(err):
		fmt.Fprintln(stderr, "sshc: the running engine did not respond")
		return exitFailure
	case refused:
		fmt.Fprintf(stderr, "sshc: %s\n", request.explain(refusal))
		return exitFailure
	case err != nil || answer.URL == "":
		fmt.Fprintln(stderr, "sshc: the engine response did not include a UI URL")
		return exitFailure
	}
	fmt.Fprintln(stdout, answer.URL)
	if open {
		// URL は既に出力済みなので、ブラウザ起動の失敗は終了コードに反映しない。
		openInBrowser(answer.URL)
	}
	return 0
}

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"sshc/internal/handoff"
)

// handoffEndpoint は、handoff が示す engine へ、handoff の秘密で認証した要求を送る相手である。
//
// 秘密（X-SSHC-CLI）を付ける要求は、すべてここから送る。client は redirect を追わない
// ものに差し替えて持つ。Go は別のホストへの redirect で Authorization や Cookie を
// 落とすが、独自のヘッダーはそのまま付けて送るためである。経路ごとに要求を組むと、
// この防御と応答の読み方が経路ごとにずれる。
type handoffEndpoint struct {
	found  handoff.Handoff
	client *http.Client
}

// handoffCall は、handoff の秘密で認証する要求 1 回分である。
type handoffCall struct {
	method string
	path   string
	// body は JSON として送る本文である。nil なら本文を付けない。
	body io.Reader
	// sessionCookie は、CLI のセッションを指す cookie である。nil なら付けない。
	sessionCookie *http.Cookie
}

// handoffAnswer は、成功した応答の読み方である。
type handoffAnswer struct {
	// status は成功として扱う status である。
	status int
	// into は応答の JSON を読む先である。nil なら本文が空であることだけを確かめる。
	into any
	// limit は応答の本文の上限である。0 なら maxEngineAPIResponse を使う。
	limit int
}

func newHandoffEndpoint(found handoff.Handoff, client *http.Client) handoffEndpoint {
	return handoffEndpoint{found: found, client: noRedirectClient(client)}
}

// send は要求を送り、応答をそのまま返す。status と本文は呼び出し側が読んで閉じる。
// 送れなかったときは、秘密を映したかもしれない本文を読み捨ててから transportProblem を返す。
func (endpoint handoffEndpoint) send(ctx context.Context, call handoffCall) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, call.method, endpoint.found.URL+call.path, call.body)
	if err != nil {
		return nil, err
	}
	request.Header.Set(handoff.HeaderName, endpoint.found.Secret)
	if call.body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if call.sessionCookie != nil {
		request.AddCookie(call.sessionCookie)
	}
	response, err := endpoint.client.Do(request)
	if err != nil {
		if response != nil {
			discardEngineResponse(response)
		}
		return nil, transportProblem(err, false)
	}
	return response, nil
}

// exchange は要求を送り、answer.status の応答だけを成功として読む。本文は上限つきで読み、
// 未知の項目も末尾の余りも許さない。status が違えば engineProblem を返す。
func (endpoint handoffEndpoint) exchange(ctx context.Context, call handoffCall, answer handoffAnswer) error {
	response, err := endpoint.send(ctx, call)
	if err != nil {
		return err
	}
	if response.StatusCode != answer.status {
		return decodeEngineProblem(response)
	}
	limit := answer.limit
	if limit == 0 {
		limit = maxEngineAPIResponse
	}
	if answer.into == nil {
		err = readEmptyResponse(response, limit)
	} else {
		err = decodeBoundedJSONResponse(response, answer.into, limit)
	}
	if err != nil && ctx.Err() != nil {
		return transportProblem(ctx.Err(), false)
	}
	return err
}

// engineRefusal は、exchange の失敗のうち、engine が違う status で断ったものを返す。
// それ以外（応答が無かった、応答を読めなかった、成功した）は false を返す。
func engineRefusal(err error) (engineProblem, bool) {
	var problem engineProblem
	if errors.As(err, &problem) && problem.Status != 0 {
		return problem, true
	}
	return engineProblem{}, false
}

// refusedRequestError は、engine が要求を断ったことを、断った理由の code を添えて述べる。
// `sshc ssh` と `sshc open` の失敗の文である。「sshc: 」は表示する側が付ける。
func refusedRequestError(refusal engineProblem) error {
	return fmt.Errorf("the engine refused the request (code %s)", refusal.Code)
}

// explainRefusedRequest は、err が engine の断りなら refusedRequestError の文にし、
// そうでなければ（nil を含む）そのまま返す。`sshc ssh` は状態と接続情報の 2 つを
// 要求するので、どちらで断られても同じ文になるよう、両方ともここを通す。
func explainRefusedRequest(err error) error {
	if refusal, refused := engineRefusal(err); refused {
		return refusedRequestError(refusal)
	}
	return err
}

package remoteengine

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
)

// relayHandler は、このマシンのブラウザからの要求を確かめ、接続先のエンジン宛てに
// 書き換えて中継する。
//
// 接続先のエンジンは、Host と Origin が自分の 127.0.0.1:<ポート> と完全に一致する要求
// だけを受け付ける。ブラウザが送るのはこのマシンの中継のポートなので、そのままでは
// 断られる。そこで、このマシンの中継のポートとの一致をここで同じ厳しさで確かめてから、
// 接続先のポートへ書き換える。確かめずに書き換えると、DNS の名前を 127.0.0.1 へ向け
// 直したよそのページの要求も、接続先のエンジンへ届いてしまう。
type relayHandler struct {
	localHost string
	proxy     *httputil.ReverseProxy
}

func newRelayHandler(localPort int, remotePort func() int, transport http.RoundTripper) http.Handler {
	handler := &relayHandler{localHost: loopbackHost(localPort)}
	cookiePrefix := relayCookiePrefix(localPort)
	handler.proxy = &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			remoteHost := loopbackHost(remotePort())
			request.SetURL(&url.URL{Scheme: "http", Host: remoteHost})
			if request.In.Header.Get("Origin") != "" {
				request.Out.Header.Set("Origin", "http://"+remoteHost)
			}
			restoreRelayedCookies(request.Out.Header, request.In.Cookies(), cookiePrefix)
		},
		ModifyResponse: func(response *http.Response) error {
			prefixSetCookies(response.Header, cookiePrefix)
			// Clear-Site-Data はポートを区別せずに 127.0.0.1 の cookie を消すので、渡すと
			// 接続先の画面のサインアウトが、このマシンの画面のセッションまで消す。
			response.Header.Del("Clear-Site-Data")
			return nil
		},
		Transport: transport,
		// ターミナルの出力やファイルの受け渡しを溜めずに流す。
		FlushInterval: -1,
		ErrorHandler: func(response http.ResponseWriter, _ *http.Request, err error) {
			writeProblem(response, http.StatusBadGateway, unreachableCode(err))
		},
	}
	return handler
}

func (h *relayHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	if request.Host != h.localHost {
		writeProblem(response, http.StatusForbidden, "invalid_host")
		return
	}
	if origin := request.Header.Get("Origin"); origin != "" && origin != "http://"+h.localHost {
		writeProblem(response, http.StatusForbidden, "cross_site_request")
		return
	}
	h.proxy.ServeHTTP(response, request)
}

// relayCookiePrefix は、接続先のエンジンが付けた cookie の名前の前に付ける文字列である。
//
// cookie はポートを区別しない。このマシンのエンジンも接続先のエンジンも、127.0.0.1 に
// 同じ名前の cookie（sshc_session）を置くので、そのままでは片方を開くともう片方の
// セッションを上書きする。上書きされた側の画面はセッションを失う。中継のポートごとの
// 名前にして、ブラウザの中で分けておく。
func relayCookiePrefix(localPort int) string {
	return "sshc-relay-" + strconv.Itoa(localPort) + "-"
}

// prefixSetCookies は、接続先のエンジンが付けた cookie の名前の前に prefix を付ける。
// Set-Cookie の値は名前から始まるので、前に足すだけで名前が変わる。
func prefixSetCookies(header http.Header, prefix string) {
	cookies := header.Values("Set-Cookie")
	header.Del("Set-Cookie")
	for _, cookie := range cookies {
		header.Add("Set-Cookie", prefix+cookie)
	}
}

// restoreRelayedCookies は、ブラウザが送った cookie のうち、この中継が名前を変えたもの
// だけを元の名前に戻して接続先へ送る。ほかの cookie（このマシンのエンジンの sshc_session
// など）は接続先へ送らない。
func restoreRelayedCookies(header http.Header, sent []*http.Cookie, prefix string) {
	header.Del("Cookie")
	var restored []string
	for _, cookie := range sent {
		if name, relayed := strings.CutPrefix(cookie.Name, prefix); relayed && name != "" {
			restored = append(restored, name+"="+cookie.Value)
		}
	}
	if len(restored) > 0 {
		header.Set("Cookie", strings.Join(restored, "; "))
	}
}

func loopbackHost(port int) string {
	return "127.0.0.1:" + strconv.Itoa(port)
}

// writeProblem は、エンジンの断りと同じ形（application/problem+json）で断る。
func writeProblem(response http.ResponseWriter, status int, code string) {
	response.Header().Set("Content-Type", "application/problem+json")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: "request rejected"})
}

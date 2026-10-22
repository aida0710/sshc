package remoteengine

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
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
	handler.proxy = &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			remoteHost := loopbackHost(remotePort())
			request.SetURL(&url.URL{Scheme: "http", Host: remoteHost})
			if request.In.Header.Get("Origin") != "" {
				request.Out.Header.Set("Origin", "http://"+remoteHost)
			}
		},
		Transport: transport,
		// ターミナルの出力やファイルの受け渡しを溜めずに流す。
		FlushInterval: -1,
		ErrorHandler: func(response http.ResponseWriter, _ *http.Request, _ error) {
			writeProblem(response, http.StatusBadGateway, "remote_engine_unreachable")
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

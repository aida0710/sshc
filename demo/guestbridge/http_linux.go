package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (b *browserBridge) fetch(request browserRequest) {
	if len(request.Body) > maxBridgeBodyBytes {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Demo requests are limited to 2 MiB"})
		return
	}
	path, err := url.ParseRequestURI(request.Path)
	if err != nil || path.IsAbs() || path.Host != "" || !strings.HasPrefix(path.Path, "/api/v1/") {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Invalid API path"})
		return
	}
	forwarded, err := http.NewRequestWithContext(context.Background(), request.Method, engineOrigin+request.Path, bytes.NewReader(request.Body))
	if err != nil {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Invalid request"})
		return
	}
	for name, value := range request.Headers {
		forwarded.Header.Set(name, value)
	}
	forwarded.Header.Set("Origin", engineOrigin)
	forwarded.Header.Set("Sec-Fetch-Site", "same-origin")
	response, err := b.client.Do(forwarded)
	if err != nil {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Engine request failed"})
		return
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBridgeBodyBytes+1))
	if err != nil {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Response read failed"})
		return
	}
	if len(body) > maxBridgeBodyBytes {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Demo responses are limited to 2 MiB"})
		return
	}
	b.reply(browserReply{ID: request.ID, Kind: "response", Status: response.StatusCode, Headers: response.Header, Body: body})
}

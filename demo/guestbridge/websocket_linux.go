package main

import (
	"context"
	"net/http"
	"net/url"

	"github.com/coder/websocket"
)

func (b *browserBridge) openSocket(request browserRequest) {
	path, err := url.ParseRequestURI(request.Path)
	if err != nil || path.IsAbs() || path.Host != "" || path.Path != "/terminal/stream" {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Invalid stream path"})
		return
	}
	socket, _, err := websocket.Dial(context.Background(), engineOrigin+request.Path, &websocket.DialOptions{
		HTTPClient: b.client, HTTPHeader: http.Header{"Origin": {engineOrigin}},
	})
	if err != nil {
		b.reply(browserReply{ID: request.ID, Kind: "error", Text: "Terminal connection failed"})
		return
	}
	// Native terminal output frames can exceed the WebSocket library's default limit.
	socket.SetReadLimit(maxBridgeBodyBytes)
	b.socketMutex.Lock()
	b.sockets[request.ID] = socket
	b.socketMutex.Unlock()
	defer func() {
		b.socketMutex.Lock()
		delete(b.sockets, request.ID)
		b.socketMutex.Unlock()
		socket.CloseNow()
		b.reply(browserReply{ID: request.ID, Kind: "closed"})
	}()
	b.reply(browserReply{ID: request.ID, Kind: "opened"})
	for {
		kind, body, err := socket.Read(context.Background())
		if err != nil {
			return
		}
		if kind == websocket.MessageText {
			b.reply(browserReply{ID: request.ID, Kind: "message", Text: string(body)})
		} else {
			b.reply(browserReply{ID: request.ID, Kind: "message", Body: body})
		}
	}
}

func (b *browserBridge) routeSocketInput(request browserRequest) {
	b.socketMutex.Lock()
	socket := b.sockets[request.ID]
	b.socketMutex.Unlock()
	if socket == nil {
		return
	}
	if request.Kind == "close" {
		socket.CloseNow()
		return
	}
	// WebSocket writes preserve the order in which serial requests arrive.
	kind, body := websocket.MessageBinary, request.Body
	if request.Text != nil {
		kind, body = websocket.MessageText, []byte(*request.Text)
	}
	_ = socket.Write(context.Background(), kind, body)
}

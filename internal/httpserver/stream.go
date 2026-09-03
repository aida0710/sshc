package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v5"

	"sshc/internal/terminal"
)

// フレームの形。
//
//   - バイナリフレーム、PTY の生バイト列。サーバー→クライアントは出力、
//     クライアント→サーバーは打鍵。base64 を挟まない。
//   - テキストフレーム、JSON の制御メッセージ。
type resizeMessage struct {
	Resize *struct {
		Cols int `json:"cols"`
		Rows int `json:"rows"`
	} `json:"resize,omitempty"`
}

type exitMessage struct {
	Exit struct {
		Code   int    `json:"code"`
		Signal string `json:"signal"`
	} `json:"exit"`
}

type replayMessage struct {
	Replay struct {
		Start     uint64 `json:"start"`
		Next      uint64 `json:"next"`
		End       uint64 `json:"end"`
		Truncated bool   `json:"truncated"`
	} `json:"replay"`
}

const (
	// maxKeystrokeFrame は、一度の打鍵として受け取る上限である。貼り付けは
	// これより大きくなりうるので、ユーザーが打つ 1 文字ではなく 1 画面分を基準にする。
	maxKeystrokeFrame = 1 << 20
	// writeTimeout は、1 フレームを書き出すのに許す時間である。これを超える
	// クライアントは読んでいない。落とすが、PTY は止めない。
	writeTimeout = 10 * time.Second
)

// Stream は、セッションひとつへ繋ぐ。
//
// 認可はチケットひとつである。使い捨てで、ひとつのセッション ID に束縛され、
// 10 秒で失効する。無効・期限切れ・使用済みのいずれも、アップグレードせずに
// 403 を返す。101 を返してから閉じると、拒否の理由がブラウザ側で
// 「繋がったのに切れた」と区別できなくなる。
func (h TerminalHandlers) Stream(c *echo.Context) error {
	request := c.Request()
	response := c.Response()

	// Origin は WebSocket でも送られるので、ここでも完全一致を確認する。
	// これはチケットの代わりではなく、その手前に置くもう一枚である。
	if request.Header.Get(echo.HeaderOrigin) != h.ExpectedOrigin {
		return c.NoContent(http.StatusForbidden)
	}
	// Sec-Fetch-Site がハンドシェイクに付くかはブラウザ次第である。付いたものは
	// 検査し、付かないものはチケットと Origin だけで判断する。付くと決め打つと
	// 送らないブラウザで端末が開けなくなり、無視すると送るブラウザで一枚失う。
	if site := request.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		return c.NoContent(http.StatusForbidden)
	}

	claim, ok := h.Tickets.Redeem(request.URL.Query().Get("ticket"))
	if !ok {
		return c.NoContent(http.StatusForbidden)
	}
	session, ok := h.Registry.Lookup(claim.SessionID)
	if !ok {
		return c.NoContent(http.StatusForbidden)
	}

	connection, err := websocket.Accept(response, request, &websocket.AcceptOptions{
		// Origin はこちらが上で確認済みである。ライブラリ側の照合は Host
		// ヘッダーとの比較であり、こちらは期待するオリジンそのものと比べている。
		InsecureSkipVerify: true,
	})
	if err != nil {
		return nil
	}
	// クライアントから届くのは打鍵と小さい制御フレームだけである。
	connection.SetReadLimit(maxKeystrokeFrame)

	h.pump(request.Context(), connection, session, claim.Cursor)
	return nil
}

// pump は、繋がっているあいだ両方向を運ぶ。
//
// coder/websocket は同時に 1 つの読み手と 1 つの書き手を許すので、打鍵は
// このゴルーチン、出力と終了は別のゴルーチンが扱う。
func (h TerminalHandlers) pump(parent context.Context, connection *websocket.Conn, session *terminal.Session, cursor uint64) {
	ctx, stop := context.WithCancel(parent)
	defer stop()

	replay, stream, ok := session.AttachFrom(cursor)
	if !ok {
		_ = connection.Close(websocket.StatusPolicyViolation, "the replay cursor is ahead")
		return
	}
	defer session.Detach(stream)

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer stop()
		// メタデータを先に送る。ブラウザは replay のバイトを既読 cursor に
		// 二重加算せず、リングから脱落した場合だけ decoder を初期化できる。
		var announced replayMessage
		announced.Replay.Start = replay.Start
		announced.Replay.Next = replay.Next
		announced.Replay.End = replay.End
		announced.Replay.Truncated = replay.Truncated
		encoded, err := json.Marshal(announced)
		if err != nil || !writeFrame(ctx, connection, websocket.MessageText, encoded) {
			return
		}
		// 再アタッチ時は、指定 cursor 以後だけを送り、その後ライブ出力へ継ぐ。
		if len(replay.Data) > 0 && !writeFrame(ctx, connection, websocket.MessageBinary, replay.Data) {
			return
		}
		for chunk := range stream.Output() {
			if !writeFrame(ctx, connection, websocket.MessageBinary, chunk) {
				return
			}
		}
		// チャンネルが閉じたのが「落とされた」ためなら、終了は起きていない。
		if stream.Dropped() {
			_ = connection.Close(websocket.StatusPolicyViolation, "the client fell behind")
			return
		}
		if info := session.Exit(); info != nil {
			var message exitMessage
			message.Exit.Code, message.Exit.Signal = info.Code, info.Signal
			if encoded, err := json.Marshal(message); err == nil {
				writeFrame(ctx, connection, websocket.MessageText, encoded)
			}
		}
		_ = connection.Close(websocket.StatusNormalClosure, "")
	}()

	for {
		kind, payload, err := connection.Read(ctx)
		if err != nil {
			break
		}
		switch kind {
		case websocket.MessageBinary:
			// 打鍵はそのまま PTY へ。終了済みへの書き込みは暗黙に捨てる。
			// 相手はもう居らず、それは切断ではないからだ。
			if _, err := session.Write(payload); err != nil && !errors.Is(err, terminal.ErrExited) {
				stop()
			}
		case websocket.MessageText:
			var message resizeMessage
			if err := json.Unmarshal(payload, &message); err != nil || message.Resize == nil {
				continue
			}
			// 範囲の外は TIOCSWINSZ へ渡さない。壊れた幅で描くより、
			// 前の幅のままの方がよい。
			_ = session.Resize(terminal.Size{
				Cols: clampDimension(message.Resize.Cols),
				Rows: clampDimension(message.Resize.Rows),
			})
		}
	}

	// 読み側が終わったので、書き側も畳む。WebSocket が切れてもセッションは
	// 死なない。同じ ID へ新しいチケットで繋ぎ直せる。
	stop()
	session.Detach(stream)
	<-writerDone
	_ = connection.CloseNow()
}

func clampDimension(value int) uint16 {
	if value < 0 || value > terminal.MaxCols {
		return 0
	}
	return uint16(value)
}

func writeFrame(ctx context.Context, connection *websocket.Conn, kind websocket.MessageType, payload []byte) bool {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return connection.Write(writeCtx, kind, payload) == nil
}

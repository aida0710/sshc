package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// engine が経路ごとに差し出す中継である。
//
// CLI（`sshc <接続先>`）とホストの ssh（`sshc vpn proxy`）は、ここへ繋ぐ。engine を
// 通すのは、どの入口からの接続も同じように数えるためである。数えられない接続が
// あると、使っている経路を無操作と見なして停止してしまう。
//
// 繋いだ側は、最初に接続先を1行（`host:port\n`）送る。engine はその接続先へ繋ぎ、
// 結果を1行の JSON（RelayReply）で返してから、両方向のバイト列を運ぶ。繋いだ側は
// 答えを読むまで何も送らない。

// engineRelaySocketName は、engine が差し出す中継の名前である。
const engineRelaySocketName = "engine.sock"

// maxSocketPathLength は、Unix ソケットのパスに書ける長さである。sun_path は
// Linux で 108 バイト、macOS で 104 バイトで、終わりの NUL に 1 バイト使う。短い
// 方に合わせる。
const maxSocketPathLength = 103

const (
	// maxRelayLineBytes は、1行目（接続先）と答えの長さの上限である。名前の上限
	// （253 字）とポートが収まる。
	maxRelayLineBytes = 300
	// relayRequestTimeout は、繋いでから1行目が届くまで待つ上限である。
	relayRequestTimeout = 10 * time.Second
	// acceptRetryLimit は、受け付けが一時的に失敗したときに待つ上限である
	// （ファイル記述子が尽きた、など）。
	acceptRetryLimit = time.Second
)

// 中継の答えのコードである。HTTP API の problem と同じ語を使い、CLI は同じ訳で見せる。
const (
	// CodeDestinationInvalid は、接続先を VPN 経由で使えないことを表す。
	CodeDestinationInvalid = "vpn_destination_invalid"
	// CodeTargetFailed は、経路はあるが接続先へ繋げなかったことを表す。
	CodeTargetFailed = "vpn_target_failed"
	// CodeSessionFailed は、経路が使えないことを表す。
	CodeSessionFailed = "vpn_session_failed"
)

// ErrSocketPath は、中継のソケットを置く場所のパスが長すぎることを表す。
var ErrSocketPath = errors.New("the vpn relay socket path is too long")

// RelayReply は、engine の中継が1行目に答える1行である。Code が空なら繋がった。
type RelayReply struct {
	Code   string `json:"code,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// relayDialer は、engine の中継が受けた接続のために、接続先への接続を開く。
type relayDialer func(ctx context.Context, address string) (net.Conn, error)

// engineRelay は、ひとつの経路の中継の待ち受けである。
type engineRelay struct {
	path     string
	listener net.Listener
	// accepting は、受け付けの goroutine が終わると閉じる。
	accepting chan struct{}
	// lifetime は、待ち受けをやめると取り消される。1行目を待っている接続と、
	// 接続先へ繋ぎに行っている接続を打ち切る。
	lifetime context.Context
	stop     context.CancelFunc
}

// openEngineRelay は、path で待ち受け、受けた接続を dial が返す接続とつなぐ。
//
// dial は、数えられる接続を返す。受けた接続が終われば、その接続も閉じる。
func openEngineRelay(path string, dial relayDialer) (*engineRelay, error) {
	if err := removeIfPresent(path); err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrSessionFailed, err)
	}
	// ディレクトリは 0700 だが、ソケットそのものも利用者だけにする。
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	lifetime, stop := context.WithCancel(context.Background())
	relay := &engineRelay{
		path: path, listener: listener, accepting: make(chan struct{}), lifetime: lifetime, stop: stop,
	}
	go relay.accept(dial)
	return relay, nil
}

func (relay *engineRelay) accept(dial relayDialer) {
	defer close(relay.accepting)
	wait := time.Duration(0)
	for {
		client, err := relay.listener.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			// 一時的な失敗で受け付けをやめると、待ち受けは開いたまま、誰も受け
			// 取らない。少し待って受け付け直す。
			wait = min(max(2*wait, 5*time.Millisecond), acceptRetryLimit)
			time.Sleep(wait)
			continue
		}
		wait = 0
		go relay.serve(client, dial)
	}
}

// serve は、1行目の接続先へ繋ぎ、答えを返してから、両方向へバイト列を運ぶ。
func (relay *engineRelay) serve(client net.Conn, dial relayDialer) {
	defer func() { _ = client.Close() }()
	_ = client.SetReadDeadline(time.Now().Add(relayRequestTimeout))
	address, err := readRelayLine(client)
	if err != nil {
		return
	}
	_ = client.SetReadDeadline(time.Time{})
	ctx, cancel := context.WithCancel(relay.lifetime)
	defer cancel()
	route, err := dial(ctx, address)
	if err != nil {
		_ = writeRelayReply(client, relayReplyFor(err))
		return
	}
	defer func() { _ = route.Close() }()
	if err := writeRelayReply(client, RelayReply{}); err != nil {
		return
	}
	pipeRelay(client, route)
}

// readRelayLine は、1行を読む。
//
// 1バイトずつ読む。まとめて読むと、改行のあとに続くバイト列まで読み込んでしまう。
func readRelayLine(client io.Reader) (string, error) {
	line := make([]byte, 0, 64)
	one := make([]byte, 1)
	for len(line) <= maxRelayLineBytes {
		if _, err := io.ReadFull(client, one); err != nil {
			return "", err
		}
		if one[0] == '\n' {
			return string(line), nil
		}
		line = append(line, one[0])
	}
	return "", errors.New("the relay line is too long")
}

// WriteRelayRequest は、繋ぐ側が1行目（接続先）を送る。
func WriteRelayRequest(writer io.Writer, address string) error {
	if len(address) > maxRelayLineBytes || bytes.ContainsAny([]byte(address), "\r\n") {
		return &DestinationError{Address: address, Reason: ReasonFormat}
	}
	_, err := io.WriteString(writer, address+"\n")
	return err
}

func writeRelayReply(writer io.Writer, reply RelayReply) error {
	encoded, err := json.Marshal(reply)
	if err != nil {
		return err
	}
	_, err = writer.Write(append(encoded, '\n'))
	return err
}

// ReadRelayReply は、繋ぐ側が engine の答えを読む。答えのあとのバイト列は読まない。
func ReadRelayReply(reader io.Reader) (RelayReply, error) {
	line, err := readRelayLine(reader)
	if err != nil {
		return RelayReply{}, err
	}
	var reply RelayReply
	if err := json.Unmarshal([]byte(line), &reply); err != nil {
		return RelayReply{}, err
	}
	return reply, nil
}

// relayReplyFor は、繋げなかった理由を答えの形へ直す。
func relayReplyFor(err error) RelayReply {
	var destination *DestinationError
	if errors.As(err, &destination) {
		return RelayReply{Code: CodeDestinationInvalid, Reason: string(destination.Reason)}
	}
	var target *TargetFailure
	if errors.As(err, &target) {
		return RelayReply{Code: CodeTargetFailed, Reason: string(target.Reason)}
	}
	var session *SessionFailure
	if errors.As(err, &session) {
		return RelayReply{Code: CodeSessionFailed, Reason: string(session.Reason)}
	}
	return RelayReply{Code: CodeSessionFailed, Reason: string(FailureUnknown)}
}

// pipeRelay は、受けた接続と経路の接続のあいだで、両方向へバイト列を運ぶ。
func pipeRelay(client, route net.Conn) {
	var copying sync.WaitGroup
	copying.Add(2)
	go func() {
		defer copying.Done()
		_, _ = io.Copy(route, client)
		closeWrite(route)
	}()
	go func() {
		defer copying.Done()
		_, _ = io.Copy(client, route)
		closeWrite(client)
	}()
	copying.Wait()
}

// closeWrite は、送る側が終わったことを相手へ伝える。伝えないと、相手は入力の
// 終わりを待ち続ける。
func closeWrite(connection net.Conn) {
	if half, ok := connection.(interface{ CloseWrite() error }); ok {
		_ = half.CloseWrite()
		return
	}
	if counted, ok := connection.(*countedConnection); ok {
		closeWrite(counted.Conn)
	}
}

// close は、待ち受けをやめ、ソケットを消す。通っている接続は、経路の側が
// 閉じれば終わる。
func (relay *engineRelay) close() error {
	relay.stop()
	err := relay.listener.Close()
	<-relay.accepting
	if removeErr := removeIfPresent(relay.path); removeErr != nil {
		return removeErr
	}
	if err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}

// requireSocketPath は、この経路の中継のソケットのパスが長さの上限に収まるかを
// 確かめる。
//
// 収まらないと、中継が分かりにくい失敗の仕方で開けない。workspace が深い場所に
// あると起こりうる。
func requireSocketPath(directory string) error {
	if path := filepath.Join(directory, engineRelaySocketName); len(path) > maxSocketPathLength {
		return fmt.Errorf("%w: %d > %d", ErrSocketPath, len(path), maxSocketPathLength)
	}
	return nil
}

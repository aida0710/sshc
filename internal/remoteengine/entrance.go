// Package remoteengine は、SSH の接続先で動いている sshc エンジンを、このマシンの
// ブラウザから開くための中継を持つ。
//
// 接続先のエンジンは、接続先の 127.0.0.1 だけで待ち受ける。そこへは SSH の接続の中を
// 通して届き、このマシンでも 127.0.0.1 だけで待ち受ける。どちらのマシンでも、外へ
// 待ち受けを開けない。
package remoteengine

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// EntranceCommand は、接続先で sshc を探し、入口の URL を 1 件発行させるコマンドである。
//
// 非対話の SSH のコマンドは、ログインシェルの設定を読まない PATH で走る。Homebrew や
// install.sh の置き先はそこに入らないことが多いので、sshc の既定の置き先を順に探す。
// alias やホスト名は埋め込まない固定の文字列で、接続先のシェルが 1 本の文字列として
// 受け取る。見つからなければ notInstalledExit で終わる。
const EntranceCommand = `sh -c 'for candidate in "$(command -v sshc)" "$HOME/.local/bin/sshc" /home/linuxbrew/.linuxbrew/bin/sshc /opt/homebrew/bin/sshc /usr/local/bin/sshc; do if [ -n "$candidate" ] && [ -x "$candidate" ]; then exec "$candidate" open --no-browser; fi; done; echo "sshc was not found" >&2; exit 127'`

// EntranceLimit は、EntranceCommand が終わるまでの上限である。接続先のエンジンが
// 止まっていれば、sshc open はエンジンを起こして準備を待つ（その上限が 20 秒）。
// それに URL の発行を足しても収まる長さにする。
const EntranceLimit = 30 * time.Second

// notInstalledExit は、EntranceCommand が sshc を見つけられなかったときの終了コードで
// ある。シェルがコマンドを見つけられなかったときと同じ値にする。
const notInstalledExit = 127

var (
	// ErrNotInstalled は、接続先に sshc が見つからなかったことを表す。
	ErrNotInstalled = errors.New("sshc was not found on the remote host")
	// ErrNoEntrance は、接続先の sshc が入口の URL を返さなかったことを表す。
	ErrNoEntrance = errors.New("sshc on the remote host did not return a UI URL")
	// ErrForwardingRefused は、接続先の SSH サーバーが、接続先のエンジンへの TCP を
	// 開かなかったことを表す。sshd_config の AllowTcpForwarding や PermitOpen で断られる。
	ErrForwardingRefused = errors.New("the SSH server refused to open a connection to the remote engine")
)

// entrance は、接続先のエンジンが発行した入口の URL である。
type entrance struct {
	// port は、接続先のエンジンが接続先の 127.0.0.1 で待ち受けるポートである。
	port int
	// rest は、URL のうちパスより後ろ（パス、クエリ、#bootstrap=… のフラグメント）である。
	rest string
}

// parseEntrance は、EntranceCommand の標準出力から入口の URL を読む。
//
// 受け付けるのは、http://127.0.0.1:<ポート>/ で始まる URL がちょうど 1 行だけの
// 出力である。それ以外の宛先へ中継すると、接続先から見て別のマシンへ届いてしまう。
func parseEntrance(stdout []byte) (entrance, error) {
	line := strings.TrimSpace(string(stdout))
	if line == "" || strings.ContainsAny(line, "\r\n") {
		return entrance{}, ErrNoEntrance
	}
	parsed, err := url.Parse(line)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Hostname() != "127.0.0.1" {
		return entrance{}, ErrNoEntrance
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return entrance{}, ErrNoEntrance
	}
	if parsed.EscapedPath() != "" && !strings.HasPrefix(parsed.EscapedPath(), "/") {
		return entrance{}, ErrNoEntrance
	}
	rest := line[len(parsed.Scheme+"://"+parsed.Host):]
	if rest == "" {
		rest = "/"
	}
	return entrance{port: port, rest: rest}, nil
}

// at は、同じ入口をこのマシンの 127.0.0.1 の port で開く URL を返す。
func (e entrance) at(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, e.rest)
}

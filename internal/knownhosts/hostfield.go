package knownhosts

import "strconv"

// DefaultPort は、known_hosts がホスト名だけで書く SSH のポートである。
const DefaultPort = 22

// HostField は、known_hosts がそのホストを書く形である。
//
// 既定のポートでは名前だけ、それ以外は [host]:port になる（OpenSSH の
// put_host_port）。鍵を書く側と照合する側がこの関数を共有する。形がずれると、
// 受け入れて書いた鍵に次の接続が一致しない。
func HostField(host string, port int) string {
	if port == DefaultPort {
		return host
	}
	return "[" + host + "]:" + strconv.Itoa(port)
}

// ParsePort は、設定に書かれたポートを数にする。"0022" も 22 として読む。
// 読めない値は既定のポートとして扱う。
func ParsePort(port string) int {
	parsed, err := strconv.Atoi(port)
	if err != nil {
		return DefaultPort
	}
	return parsed
}

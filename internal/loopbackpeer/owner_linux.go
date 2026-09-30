//go:build linux && !android

package loopbackpeer

import (
	"bufio"
	"encoding/binary"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

// socketTables は、TCP のソケットの一覧。IPv6 のソケットから IPv4 射影アドレスで
// つなぐこともできるので、両方を見る。IPv4 だけを見ると、それで確認をすり抜けられる。
var socketTables = []string{"/proc/net/tcp", "/proc/net/tcp6"}

// ソケットの一覧の各行で、空白で区切った何番目の欄に何があるか。
const (
	localAddressField  = 1
	remoteAddressField = 2
	stateField         = 3
	uidField           = 7
	inodeField         = 9
)

// establishedState は、一覧の st の欄で TCP_ESTABLISHED を表す値。
const establishedState = "01"

// procWordHexDigits は、一覧がアドレスを 32 bit ずつ 16 進で書くときの桁数。
const procWordHexDigits = 8

// 一覧がアドレス全体を書く 16 進の桁数。1 byte を 2 桁で書く。
const (
	ipv4HexDigits = net.IPv4len * 2
	ipv6HexDigits = net.IPv6len * 2
)

// rootUID は root の uid。root はもともと何でも読めるので、相手が root でも拒まない。
// WSL の localhost 転送のように、root の中継がブラウザの代わりにつなぐこともある。
const rootUID = 0

// MayBelongToAnotherUser は、client から server への TCP 接続が、このプロセスとも
// root とも違う OS ユーザーのものかもしれないときに true を返す。
//
// 持ち主を読めるのは、client 側のソケットが開いている間だけである。閉じると、行は
// timewait か孤立したソケットになって inode の欄が 0 になり、uid の欄は 0（root）か
// 閉じる前の uid になる。RST で閉じれば行ごと消える。どの場合も server は届いていた
// 要求をまだ読めるので、要求を送ってすぐ閉じれば root として通れてしまう。そこで、
// 次のどちらかなら、持ち主を確かめられないものとして true を返す。
//
//   - client 側の行が ESTABLISHED でないか、inode が 0。
//   - client 側の行が無く、server 側の行も ESTABLISHED で無い（RST で閉じた）。
//
// client 側の行が無くても server 側の行が ESTABLISHED なら、相手はこのネットワーク
// 名前空間の外にいる（WSL の mirrored ネットワーク）ので false を返す。同じマシンの
// 別 OS ユーザーが開いたままのソケットは、必ず一覧に載る。一覧をどちらも読めない
// ときは確かめられないので false を返す。
func MayBelongToAnotherUser(client, server netip.AddrPort) bool {
	connection := tcpConnection{client: unmapped(client), server: unmapped(server)}
	readable := false
	for _, path := range socketTables {
		if connection.readTableFile(path) {
			readable = true
		}
	}
	return readable && connection.mayBelongToAnotherUser()
}

// socketRow は、ソケットの一覧の 1 行のうち、持ち主を確かめるのに使う欄。
type socketRow struct {
	state string
	owner uint32
	inode uint64
}

// open は、ソケットをまだプロセスが持っていて、接続も切れていないかを返す。
func (row socketRow) open() bool {
	return row.state == establishedState && row.inode != 0
}

// tcpConnection は、1 本の TCP 接続と、一覧で見つけたその両端の行。
type tcpConnection struct {
	client, server netip.AddrPort
	// clientSide は local が client で remote が server の行、serverSide はその逆の行。
	// 一覧に無ければ nil。
	clientSide, serverSide *socketRow
}

func (connection *tcpConnection) mayBelongToAnotherUser() bool {
	if connection.clientSide != nil {
		return !connection.clientSide.open() || isAnotherUser(connection.clientSide.owner)
	}
	return connection.serverSide == nil || !connection.serverSide.open()
}

func isAnotherUser(owner uint32) bool {
	return owner != uint32(os.Getuid()) && owner != rootUID
}

// readTableFile は、一覧のファイルから接続の両端の行を探す。読めたかどうかを返す。
func (connection *tcpConnection) readTableFile(path string) bool {
	table, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = table.Close() }()
	connection.readTable(table)
	return true
}

// readTable は、/proc/net/tcp と同じ形の一覧から、接続の client 側と server 側の行を
// 探す。2 つは local と remote が逆の行なので、取り違えない。
func (connection *tcpConnection) readTable(table io.Reader) {
	lines := bufio.NewScanner(table)
	lines.Scan() // 見出しの行
	for lines.Scan() {
		fields := strings.Fields(lines.Text())
		if len(fields) <= inodeField {
			continue
		}
		local, localOK := parseSocketAddress(fields[localAddressField])
		remote, remoteOK := parseSocketAddress(fields[remoteAddressField])
		if !localOK || !remoteOK {
			continue
		}
		switch {
		case local == connection.client && remote == connection.server:
			connection.clientSide = parseSocketRow(fields)
		case local == connection.server && remote == connection.client:
			connection.serverSide = parseSocketRow(fields)
		}
	}
}

// parseSocketRow は、行の欄を読む。uid か inode を読めない行は、持ち主を確かめられない
// 閉じたソケットと同じに扱う。
func parseSocketRow(fields []string) *socketRow {
	owner, ownerErr := strconv.ParseUint(fields[uidField], 10, 32)
	inode, inodeErr := strconv.ParseUint(fields[inodeField], 10, 64)
	if ownerErr != nil || inodeErr != nil {
		return &socketRow{}
	}
	return &socketRow{state: fields[stateField], owner: uint32(owner), inode: inode}
}

// parseSocketAddress は、一覧の "0100007F:1F90" のようなアドレスを読む。
//
// カーネルはネットワークバイト順のアドレスを 32 bit ずつ、このマシンのバイト順の
// 整数として 16 進で書く。ポートはこのマシンの整数として書く。
func parseSocketAddress(field string) (netip.AddrPort, bool) {
	addressHex, portHex, ok := strings.Cut(field, ":")
	if !ok || (len(addressHex) != ipv4HexDigits && len(addressHex) != ipv6HexDigits) {
		return netip.AddrPort{}, false
	}
	address := make([]byte, 0, len(addressHex)/2)
	for start := 0; start < len(addressHex); start += procWordHexDigits {
		word, err := strconv.ParseUint(addressHex[start:start+procWordHexDigits], 16, 32)
		if err != nil {
			return netip.AddrPort{}, false
		}
		address = binary.NativeEndian.AppendUint32(address, uint32(word))
	}
	port, err := strconv.ParseUint(portHex, 16, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}
	parsed, ok := netip.AddrFromSlice(address)
	if !ok {
		return netip.AddrPort{}, false
	}
	return unmapped(netip.AddrPortFrom(parsed, uint16(port))), true
}

// unmapped は、IPv4 射影アドレスを IPv4 に直す。同じ接続を IPv4 と IPv6 の
// どちらの書き方で受け取っても比べられるようにする。
func unmapped(address netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(address.Addr().Unmap(), address.Port())
}

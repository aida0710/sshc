//go:build linux && !android

package loopbackpeer

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tableHeader = "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"

// 一覧の st の欄の値。カーネルの TCP の状態の番号である。
const (
	finWait1State  = "04"
	finWait2State  = "05"
	closeWaitState = "08"
)

// liveInode は、プロセスが持っているソケットの行に載る、0 でない inode。
const liveInode = 12345

// tableAddress は、カーネルが一覧に書くのと同じ形でアドレスを書く。
func tableAddress(address netip.AddrPort) string {
	raw := address.Addr().AsSlice()
	var written strings.Builder
	for start := 0; start < len(raw); start += 4 {
		fmt.Fprintf(&written, "%08X", binary.NativeEndian.Uint32(raw[start:start+4]))
	}
	fmt.Fprintf(&written, ":%04X", address.Port())
	return written.String()
}

// row は、一覧の 1 行に書く値。
type row struct {
	local, remote netip.AddrPort
	state         string
	owner         uint32
	inode         uint64
}

func (written row) String() string {
	return fmt.Sprintf("   0: %s %s %s 00000000:00000000 00:00000000 00000000 %5d        0 %d 1 0000000000000000 20 4 30 10 -1\n",
		tableAddress(written.local), tableAddress(written.remote), written.state, written.owner, written.inode)
}

var (
	testClient = netip.MustParseAddrPort("127.0.0.1:54321")
	testServer = netip.MustParseAddrPort("127.0.0.1:47001")
	selfUID    = uint32(os.Getuid())
)

// openClientRow と openServerRow は、両端が開いている接続の行。
func openClientRow(owner uint32) row {
	return row{local: testClient, remote: testServer, state: establishedState, owner: owner, inode: liveInode}
}

func openServerRow() row {
	return row{local: testServer, remote: testClient, state: establishedState, owner: selfUID, inode: liveInode}
}

// anotherUID は、このプロセスとも root とも違う uid。
func anotherUID() uint32 {
	if selfUID+1 == rootUID {
		return selfUID + 2
	}
	return selfUID + 1
}

func readConnection(table string) tcpConnection {
	connection := tcpConnection{client: testClient, server: testServer}
	connection.readTable(strings.NewReader(tableHeader + table))
	return connection
}

func TestReadTableTellsTheClientSideRowFromTheServerSideRow(t *testing.T) {
	connection := readConnection(openServerRow().String() + openClientRow(1001).String())

	if connection.clientSide == nil || connection.clientSide.owner != 1001 {
		t.Fatalf("client side = %+v, want the row owned by 1001", connection.clientSide)
	}
	if connection.serverSide == nil || connection.serverSide.owner != selfUID {
		t.Fatalf("server side = %+v, want the row owned by %d", connection.serverSide, selfUID)
	}
}

func TestReadTableMatchesAnIPv4MappedRowFromTheIPv6Table(t *testing.T) {
	mapped := openClientRow(1001)
	mapped.local = netip.AddrPortFrom(netip.AddrFrom16(testClient.Addr().As16()), testClient.Port())
	mapped.remote = netip.AddrPortFrom(netip.AddrFrom16(testServer.Addr().As16()), testServer.Port())

	connection := readConnection(mapped.String())

	if connection.clientSide == nil || connection.clientSide.owner != 1001 {
		t.Fatalf("client side = %+v, want the row owned by 1001", connection.clientSide)
	}
}

func TestReadTableIgnoresRowsOfOtherConnections(t *testing.T) {
	other := openClientRow(1001)
	other.local = netip.MustParseAddrPort("127.0.0.1:40000")

	connection := readConnection(other.String() + "   1: not a row\n")

	if connection.clientSide != nil || connection.serverSide != nil {
		t.Fatalf("sides = %+v, %+v; want neither", connection.clientSide, connection.serverSide)
	}
}

func TestWhetherAConnectionMayBelongToAnotherUser(t *testing.T) {
	closedClient := func(state string, owner uint32) row {
		return row{local: testClient, remote: testServer, state: state, owner: owner}
	}
	closeWaitServer := openServerRow()
	closeWaitServer.state = closeWaitState
	for _, test := range []struct {
		name  string
		table string
		want  bool
	}{
		{name: "an open connection of this user is accepted",
			table: openClientRow(selfUID).String() + openServerRow().String(), want: false},
		{name: "an open connection of root is accepted",
			table: openClientRow(rootUID).String() + openServerRow().String(), want: false},
		{name: "an open connection of another user is refused",
			table: openClientRow(anotherUID()).String() + openServerRow().String(), want: true},
		{name: "a client closed into timewait shows root and is refused",
			table: closedClient(finWait2State, rootUID).String() + closeWaitServer.String(), want: true},
		{name: "an orphaned client socket without an inode is refused",
			table: closedClient(finWait1State, selfUID).String() + closeWaitServer.String(), want: true},
		{name: "a client row whose uid cannot be read is refused",
			table: strings.Replace(openClientRow(selfUID).String(), fmt.Sprintf("%5d", selfUID), "  bad", 1), want: true},
		{name: "a connection reset by the client leaves no rows and is refused",
			table: "", want: true},
		{name: "a peer outside this network namespace leaves only an open server row and is accepted",
			table: openServerRow().String(), want: false},
		{name: "a missing client row with a closing server row is refused",
			table: closeWaitServer.String(), want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			connection := readConnection(test.table)
			if got := connection.mayBelongToAnotherUser(); got != test.want {
				t.Fatalf("mayBelongToAnotherUser = %v, want %v\n%s", got, test.want, test.table)
			}
		})
	}
}

func TestOnlyUsersOtherThanThisProcessAndRootCountAsAnotherUser(t *testing.T) {
	if isAnotherUser(selfUID) {
		t.Error("this process's own uid counted as another user")
	}
	if isAnotherUser(rootUID) {
		t.Error("root counted as another user")
	}
	if !isAnotherUser(anotherUID()) {
		t.Errorf("uid %d did not count as another user", anotherUID())
	}
}

func TestAConnectionIsNotRefusedWhenNoSocketTableCanBeRead(t *testing.T) {
	original := socketTables
	t.Cleanup(func() { socketTables = original })
	socketTables = []string{filepath.Join(t.TempDir(), "missing")}

	if MayBelongToAnotherUser(testClient, testServer) {
		t.Fatal("MayBelongToAnotherUser = true without any readable socket table")
	}
}

// dialLoopback は、このプロセスの中でループバックの接続を張り、client と server の
// アドレスを返す。
func dialLoopback(t *testing.T) (net.Conn, netip.AddrPort, netip.AddrPort) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	connection, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection,
		netip.MustParseAddrPort(connection.LocalAddr().String()),
		netip.MustParseAddrPort(listener.Addr().String())
}

// 実際のカーネルの一覧で、この環境のバイト順と欄の並びを読めることを確かめる。
func TestAnOpenConnectionFromThisProcessIsNotFromAnotherUser(t *testing.T) {
	_, client, server := dialLoopback(t)

	if MayBelongToAnotherUser(client, server) {
		t.Fatal("MayBelongToAnotherUser = true for this process's own open connection")
	}
}

// 閉じたソケットは、行が残っていても持ち主を読めない。自分の接続でも断る。
func TestAConnectionWhoseClientHasClosedMayBelongToAnotherUser(t *testing.T) {
	connection, client, server := dialLoopback(t)
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}

	if !MayBelongToAnotherUser(client, server) {
		t.Fatal("MayBelongToAnotherUser = false after the client closed its socket")
	}
}

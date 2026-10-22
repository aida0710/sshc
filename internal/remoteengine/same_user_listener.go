package remoteengine

import (
	"net"
	"net/netip"
)

// sameUserListener は、このマシンの別の OS ユーザーが張ったかもしれない接続を、
// 1 バイトも読まずに閉じる。
//
// 接続先のエンジンは、入口の URL を最初に示した相手のブラウザを登録する。そのとき
// エンジンは接続の持ち主を確かめるが、中継を通った接続は、接続先では SSH の
// サーバー（自分の OS ユーザー）から来たように見える。このマシンの別の OS ユーザーを
// 断れるのは、ここだけである。
type sameUserListener struct {
	net.Listener
	// mayBelongToAnotherUser は、client から server への TCP 接続が、別の OS ユーザーの
	// ものかもしれないかを返す。持ち主を読めない接続も、そうかもしれないものに含める。
	mayBelongToAnotherUser func(client, server netip.AddrPort) bool
}

func (l sameUserListener) Accept() (net.Conn, error) {
	for {
		connection, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if !l.refuses(connection) {
			return connection, nil
		}
		_ = connection.Close()
	}
}

func (l sameUserListener) refuses(connection net.Conn) bool {
	if l.mayBelongToAnotherUser == nil {
		return false
	}
	client, clientErr := netip.ParseAddrPort(connection.RemoteAddr().String())
	server, serverErr := netip.ParseAddrPort(connection.LocalAddr().String())
	if clientErr != nil || serverErr != nil {
		return true
	}
	return l.mayBelongToAnotherUser(client, server)
}

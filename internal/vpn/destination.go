package vpn

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

// 経路の先で繋ぐ相手（接続先）を確かめる。
//
// 接続先はプロファイルには無い。プロファイルを付けた接続の HostName と Port から
// 決まり、接続のたびに渡される。

// ErrDestination は、接続先を VPN 経由で使えないことを表す。
var ErrDestination = errors.New("the destination cannot be reached through a vpn")

// DestinationError は、接続先を使えない理由である。
type DestinationError struct {
	// Address は、接続先の表記（`host:port`）である。
	Address string
	Reason  Reason
}

func (failure *DestinationError) Error() string {
	return fmt.Sprintf("%v: %s: %s", ErrDestination, failure.Address, failure.Reason)
}

func (failure *DestinationError) Unwrap() error { return ErrDestination }

// Destination は、address（`host:port`）を、この経路の先で繋ぐ相手として確かめる。
//
// 接続先は IPv4 アドレスか、名前である。名前は、コンテナの中でこの経路の DNS
// サーバーだけを使って名前解決する。どちらの名前空間で名前解決するのかが決まらない
// まま名前を許すと、ホストで名前解決した別のマシンへ繋ぎうる。だから名前で書いた
// 接続先には、プロファイルの DNS サーバーが要る。
func (profile Profile) Destination(address string) (Endpoint, error) {
	refuse := func(reason Reason) (Endpoint, error) {
		return Endpoint{}, &DestinationError{Address: address, Reason: reason}
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return refuse(ReasonFormat)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return refuse(ReasonOutOfRange)
	}
	if literal, err := netip.ParseAddr(host); err == nil {
		if !literal.Is4() {
			return refuse(ReasonNotIPv4)
		}
		if literal.IsUnspecified() || literal.IsLoopback() || literal.IsMulticast() {
			return refuse(ReasonUnroutable)
		}
		return Endpoint{Host: host, Port: port}, nil
	}
	if !validHostName(host) {
		return refuse(ReasonFormat)
	}
	if len(profile.DNS) == 0 {
		return refuse(ReasonNameNeedsDNS)
	}
	return Endpoint{Host: host, Port: port}, nil
}

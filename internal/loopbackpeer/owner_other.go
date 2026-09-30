//go:build !linux || android

package loopbackpeer

import "net/netip"

// MayBelongToAnotherUser は、この OS では確かめないので常に false を返す。
func MayBelongToAnotherUser(client, server netip.AddrPort) bool { return false }

package mobile

// **このファイルは使い捨てである。** Android で 2 つのことが成立するかを測る
// ためだけに存在し、答えが出たら消す。リポジトリに残してはならない。
//
// 測っているのは、ホスト上のテストでは決して分からない 2 つである。
//
//  1. 名前が引けるか。Android には /etc/resolv.conf が無く、名前解決は netd を
//     通る。cgo リゾルバがそこへ届いていなければ、このアプリケーションは
//     どのホストへも繋げない。
//  2. 開けるログインシェルがあるか。Android に /bin は無い。

import (
	"context"
	"net"
	"strings"
	"time"

	"sshc/internal/platform"
)

func ProbeDNS(host string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "OK: " + strings.Join(addresses, ",")
}

func ProbeShell() string {
	shell, err := platform.LoginShell(func(string) (string, bool) { return "", false })
	if err != nil {
		return "FAIL: " + err.Error()
	}
	return "OK: " + shell
}

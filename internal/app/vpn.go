package app

import (
	"context"
	"errors"
	"fmt"
	"net"

	"sshc/internal/application"
	"sshc/internal/secret"
	"sshc/internal/vpn"
)

// vpnStateDirectory は、中継のソケットを置く engine 専用ディレクトリである。
// ワークスペースの中にあるが、同期からは外してある。socket は運べない。
const vpnStateDirectory = "sshc/vpn"

// errVPNTargetMismatch は、接続設定の接続先とプロファイルの接続先が食い違う
// ことを表す。
//
// コンテナはプロファイルの接続先ひとつだけを通す。食い違ったまま繋ぐと、利用者が
// 設定に書いた相手ではなく、プロファイルに書いた相手へ届く。どちらが正しいかを
// 推測せず、断る。
var errVPNTargetMismatch = errors.New("the connection and its VPN profile name different targets")

// vpnRoute は、プロファイル名から設定と秘密を集め、その経路で接続先へ繋ぐ。
func vpnRoute(
	config *application.Service,
	secrets *secret.Service,
	sessions *vpn.Manager,
) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, name, address string) (net.Conn, error) {
		profile, err := config.VPNProfile(name)
		if err != nil {
			return nil, err
		}
		if profile.Target.Address() != address {
			return nil, fmt.Errorf("%w: %s は %s へ繋ぐ経路である", errVPNTargetMismatch, name, profile.Target.Address())
		}
		stored, err := secrets.VPNSecrets(name)
		if err != nil {
			return nil, err
		}
		values, err := vpn.DecodeSecrets(stored)
		if err != nil {
			return nil, err
		}
		return sessions.Dial(ctx, profile, values)
	}
}

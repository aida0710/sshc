#!/bin/sh
# 接続1本ぶんの中継。engine が docker exec -i で起動し、標準入出力を接続として使う。
#
#   connect <host> <port>
#
# 初めての接続先なら、VPNの中で名前解決し、経路とパケットフィルタを足してから
# 中継する。足すのは接続に使った接続先だけで、VPNの向こうのネットワーク全体は
# 引き込まない。
#
# 標準出力はデータの通り道なので、案内も診断も書かない。engine へは標準エラーで
# 伝える。
#   sshc-vpn-failure: <語>   中継を始められなかった理由（internal/vpn/failure.go）
#   socat の "starting data transfer loop"   接続先へ繋がり、中継を始めた合図
set -eu

runtime=/run/sshc-vpn
backend_directory=/usr/local/lib/sshc-vpn
# route.env は agent がトンネルを張り終えてから書く。無ければ経路はまだ無いか、
# もう無い。
route="$runtime/route.env"
# routed は、経路とパケットフィルタを足し終えた接続先のアドレスの控えである。
routed="$runtime/routed"
# names は、名前解決した接続先の控えである。同じ経路の中では同じアドレスを使う。
# 途中で別のアドレスへ変わると、足した経路とパケットフィルタが合わなくなる。
names="$runtime/names"

# 接続先へ繋ぐのを待つ上限（秒）。応えない相手でも、SSH の接続のタイムアウトより
# 先に理由を返す。
connect_timeout_seconds=20

host=$1
port=$2

fail() {
	printf 'sshc-vpn-failure: %s\n' "$1" >&2
	exit 1
}

if [ ! -f "$route" ]; then
	fail tunnel_lost
fi
. "$route"
. "$backend_directory/backend-$backend.sh"

# 同時に2本来ても、経路とパケットフィルタは1回だけ足す。
exec 9>"$runtime/route.lock"
flock 9

case "$host" in
*[!0-9.]*)
	# 名前は、この経路のDNSサーバーだけで名前解決する。ホストで名前解決すると、
	# 同じ名前が指す別のマシンへ繋ぎうる。
	if [ -z "$resolvers" ]; then
		fail target_needs_dns
	fi
	address=$(awk -v name="$host" '$1 == name { print $2; exit }' "$names" 2>/dev/null || true)
	if [ -z "$address" ]; then
		address=$(getent ahostsv4 "$host" | awk 'NR==1{print $1}')
		if [ -z "$address" ]; then
			fail target_unresolved
		fi
		printf '%s %s\n' "$host" "$address" >>"$names"
		# docker logs に残す。engine が「ログ」で見せる。
		echo "接続先 $host のアドレスは $address です。" >/proc/1/fd/1 2>/dev/null || true
	fi
	;;
*)
	address=$host
	;;
esac

if ! grep -qx "$address" "$routed" 2>/dev/null; then
	# VPNサーバーそのものを接続先にしない。トンネルの外側と内側が同じ相手になり、
	# 経路とパケットフィルタが互いを打ち消す。
	if [ "$address" = "${server_address:-}" ]; then
		fail target_is_server
	fi
	backend_allow "$address"
	# 接続先への経路は、このコンテナのトンネルの中にしか作らない。
	ip route replace "$address/32" dev "$interface"
	# トンネル以外から接続先へ出ようとする通信は拒否する。トンネルが落ちている
	# あいだ、接続先への通信がDockerの通常のネットワークへ流れることはない。
	iptables -A OUTPUT -d "$address" ! -o "$interface" -j REJECT
	echo "$address" >>"$routed"
	echo "接続先 $address への経路を追加しました。" >/proc/1/fd/1 2>/dev/null || true
fi

flock -u 9
exec 9>&-

# -d -d は、繋がったときの "starting data transfer loop" を標準エラーへ出させる。
# engine はこれを見て、中継を始められたと判断する。
exec socat -d -d STDIO "TCP:$address:$port,connect-timeout=$connect_timeout_seconds"

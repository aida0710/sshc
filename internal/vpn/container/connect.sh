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
#   sshc-vpn-note: <文>      行ったこと（engine が接続ログの debug2 に写す）
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

# connect が待つ上限（秒）。3つの合計に、docker exec の起動などの余裕
# （internal/vpn/target.go の connectStartAllowance）を足しても、engine が待つ上限
# （targetConnectTimeout）より短くし、engine が打ち切る前に理由を返す。
#
# route_lock_seconds は、経路とパケットフィルタを足す鍵を待つ上限である。鍵の中では
# 名前解決しないので、同時に来たほかの接続が足し終えるのを待つだけである。
route_lock_seconds=3
# resolve_timeout_seconds は、接続先の名前解決を待つ上限である。agent.sh の resolv.conf
# は、DNS サーバー1台を2秒・1回だけ待つので、3台（上限）がどれも応えなくても6秒で終わる。
resolve_timeout_seconds=6
# connect_timeout_seconds は、接続先へ繋ぐのを待つ上限である。応えない相手でも、SSH の
# 接続のタイムアウトより先に理由を返す。
connect_timeout_seconds=17

host=$1
port=$2

fail() {
	printf 'sshc-vpn-failure: %s\n' "$1" >&2
	exit 1
}

# note は、行ったことを engine の接続ログとコンテナのログの両方へ残す。
note() {
	printf 'sshc-vpn-note: %s\n' "$*" >&2
	echo "$*" >/proc/1/fd/1 2>/dev/null || true
}

if [ ! -f "$route" ]; then
	fail tunnel_lost
fi
. "$route"
. "$backend_directory/backend-$backend.sh"

# named_address は、名前解決した接続先の控えから、$1 のアドレスを返す。無ければ空。
named_address() {
	awk -v name="$1" '$1 == name { print $2; exit }' "$names" 2>/dev/null || true
}

# resolved_now は、この接続で名前解決したか（控えに無かったか）である。
resolved_now=false
case "$host" in
*[!0-9.]*)
	# 名前は、この経路のDNSサーバーだけで名前解決する。ホストで名前解決すると、
	# 同じ名前が指す別のマシンへ繋ぎうる。
	if [ -z "$resolvers" ]; then
		fail target_needs_dns
	fi
	address=$(named_address "$host")
	if [ -z "$address" ]; then
		# 名前解決は鍵の外で行う。鍵の中で待つと、IPアドレスの接続先や別の名前の
		# 接続まで、応えないDNSサーバーの後ろに並ぶ。
		note "接続先${host}をVPN内のDNSサーバーで名前解決します。"
		address=$(timeout "$resolve_timeout_seconds" getent ahostsv4 -- "$host" | awk 'NR==1{print $1}')
		if [ -z "$address" ]; then
			fail target_unresolved
		fi
		resolved_now=true
	else
		note "接続先${host}は名前解決済みです：${address}"
	fi
	;;
*)
	# engine は4つの10進数で書いたアドレスだけを渡す。それでも略記（10.1）が届いた
	# ときに経路・パケットフィルタ・socat で読み方が割れないよう、socat と同じ
	# getaddrinfo で読んだ値を3つとも使う。
	address=$(getent ahostsv4 "$host" | awk 'NR==1{print $1}')
	if [ -z "$address" ]; then
		fail target_unresolved
	fi
	;;
esac

# 同時に2本来ても、名前の控えと、経路とパケットフィルタは1回だけ足す。
exec 9>"$runtime/route.lock"
if ! flock -w "$route_lock_seconds" 9; then
	fail timeout
fi

if [ "$resolved_now" = true ]; then
	# 鍵を待つあいだに、ほかの接続が同じ名前を控えたかもしれない。同じ経路の中では同じ
	# アドレスを使うので、控えがあればそちらを使う。
	recorded=$(named_address "$host")
	if [ -z "$recorded" ]; then
		printf '%s %s\n' "$host" "$address" >>"$names"
		note "接続先${host}を名前解決しました：${address}"
	else
		address=$recorded
		note "接続先${host}は名前解決済みです：${address}"
	fi
fi

if ! grep -qx "$address" "$routed" 2>/dev/null; then
	# VPNサーバーそのものを接続先にしない。トンネルの外側と内側が同じ相手になり、
	# 経路とパケットフィルタが互いを打ち消す。サーバーが複数あれば空白で区切って並ぶ。
	case " ${server_address:-} " in
	*" $address "*)
		fail target_is_server
		;;
	esac
	backend_allow "$address"
	# 接続先への経路は、このコンテナのトンネルの中にしか作らない。
	ip route replace "$address/32" dev "$interface"
	# トンネル以外から接続先へ出ようとする通信は拒否する。トンネルが落ちている
	# あいだ、接続先への通信がDockerの通常のネットワークへ流れることはない。
	iptables -A OUTPUT -d "$address" ! -o "$interface" -j REJECT
	echo "$address" >>"$routed"
	note "接続先${address}へのルートとパケットフィルタを追加しました（インターフェース：${interface}）。"
else
	note "接続先${address}へのルートは追加済みです。"
fi
note "接続先${address}:${port}へTCPで接続します。"

flock -u 9
exec 9>&-

# -d -d は、繋がったときの "starting data transfer loop" を標準エラーへ出させる。
# engine はこれを見て、中継を始められたと判断する。
exec socat -d -d STDIO "TCP:$address:$port,connect-timeout=$connect_timeout_seconds"

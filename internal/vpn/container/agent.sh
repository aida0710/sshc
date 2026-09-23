#!/bin/sh
# ひとつのトンネルを張り、その中にある接続先への中継を差し出す。
#
# 秘密は引数にも環境変数にもイメージにも置かない。engineが docker exec の標準
# 入力で設定を書き込み、このスクリプトはそれが現れるのを待つ。読み終えた設定は
# 消す。設定はtmpfsの上にしか存在しない。
#
# backend ごとの違いは backend-<名前>.sh に置き、ここは次の関数だけを呼ぶ。
#   backend_read   設定から自分の節を読む（このあと設定は消える）
#   backend_up     トンネルを張り、interface を決める
#   backend_ready  トンネルの相手と話せたことを確かめる
#   backend_allow  名前を引いて分かった接続先を、トンネルが運ぶ相手に足す
#   backend_alive  トンネルが生きているかを返す
#   backend_down   相手へ切断を伝えて畳む
set -eu

runtime=/run/sshc-vpn
socket_directory=/run/sshc-vpn-socket
backend_directory=/usr/local/lib/sshc-vpn
profile="$runtime/profile.json"

# engineがコンテナを起動してから設定を書き込むまでの猶予。これを過ぎたら、
# 書き込む側が落ちたということなので、待ち続けずに終わる。
profile_wait_seconds=30

# トンネルが生きているかを見に行く間隔。短くしても落ちた瞬間が早く分かるだけで、
# 既に張られている接続は切れている。
tunnel_check_seconds=5

# 止める合図を受けてから、backend が相手へ切断を伝え終わるまで待つ上限。docker
# stop の猶予（10秒）より短くする。
shutdown_seconds=5

umask 077
mkdir -p "$runtime"

# pause は、合図を受けたらすぐ起きる sleep である。sh は前面の sleep が終わる
# まで trap を実行しないので、背後で眠ってそれを待つ。
pause() {
	sleep "$1" &
	wait $! || true
}

# fail は、理由の語を engine が読める場所へ書き、理由の文を標準エラーへ出して
# 終わる。語は internal/vpn/failure.go の FailureReason と同じものだけを使う。
fail() {
	reason=$1
	shift
	echo "$*" >&2
	printf '{"reason":"%s"}\n' "$reason" >"$socket_directory/failure.json" 2>/dev/null || true
	chmod 644 "$socket_directory/failure.json" 2>/dev/null || true
	stop_relay
	backend_down 2>/dev/null || true
	exit 1
}

# remaining_seconds は、engine が決めた締め切りまでの残りの秒数である。
remaining_seconds() {
	left=$((deadline - $(date +%s)))
	if [ "$left" -lt 0 ]; then
		left=0
	fi
	echo "$left"
}

# wait_for_address は、interface にアドレスが付くまで、締め切りまで待つ。
wait_for_address() {
	while ! ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '; do
		if [ "$(remaining_seconds)" -le 0 ]; then
			return 1
		fi
		pause 1
	done
}

relay=
stop_relay() {
	if [ -n "$relay" ]; then
		kill "$relay" 2>/dev/null || true
		wait "$relay" 2>/dev/null || true
		relay=
	fi
}

# 止める合図（docker stop）を受けたら、相手へ切断を伝えてから終わる。伝えずに
# 終わると、装置の側にセッションが残る。
shutdown() {
	trap - TERM INT
	stop_relay
	backend_down 2>/dev/null || true
	exit 0
}
trap shutdown TERM INT

# backend を読み込む前に合図を受けても困らないよう、何もしない既定を置く。
backend_down() { :; }

seconds=0
while [ ! -f "$profile" ]; do
	if [ "$seconds" -ge "$profile_wait_seconds" ]; then
		echo "設定を受け取れないまま時間切れになりました。" >&2
		exit 1
	fi
	pause 1
	seconds=$((seconds + 1))
done

backend=$(jq -r '.backend' "$profile")
target_host=$(jq -r '.target.host' "$profile")
target_port=$(jq -r '.target.port' "$profile")
socket_owner=$(jq -r '.socketOwner' "$profile")
# VPNの中で名前を引くDNSサーバー。空なら、接続先はアドレスで書かれている。
resolvers=$(jq -r 'if .dns then .dns[] else empty end' "$profile" | tr '\n' ' ')
# 応えない相手を待つ締め切り（UNIX 秒）。engine が待つのをやめるより先に諦め、
# どこで止まったかをログへ残す。値は engine が決める。
deadline=$(jq -r '.deadline' "$profile")

case "$backend" in
wireguard | l2tp_ipsec | openconnect)
	. "$backend_directory/backend-$backend.sh"
	;;
*)
	rm -f "$profile"
	echo "対応していない方式です: $backend" >&2
	exit 1
	;;
esac

backend_read
rm -f "$profile"
backend_up
backend_ready

# VPNの中のDNSは、この経路の中だけで使う。問い合わせもトンネルの中にしか出さ
# ない。ホストのresolv.confもDockerの既定のDNSも、このコンテナの外では変わらない。
if [ -n "$resolvers" ]; then
	for resolver in $resolvers; do
		ip route replace "$resolver/32" dev "$interface"
		iptables -A OUTPUT -d "$resolver" ! -o "$interface" -j REJECT
	done
	: >"$runtime/resolv.conf"
	for resolver in $resolvers; do
		printf 'nameserver %s\n' "$resolver" >>"$runtime/resolv.conf"
	done
	# /etc/resolv.conf は Docker の bind mount である。置き換えられないので、
	# 中身だけを書き換える。
	cat "$runtime/resolv.conf" >/etc/resolv.conf
fi

# 接続先が名前なら、VPNの中で引く。ホストで引くと、同じ名前が指す別の機械へ
# 繋ぎうる。引けたアドレスだけが、この経路が触ってよい相手である。
case "$target_host" in
*[!0-9.]*)
	target_address=$(getent ahostsv4 "$target_host" | awk 'NR==1{print $1}')
	if [ -z "$target_address" ]; then
		fail target_unresolved "VPN内で接続先の名前解決に失敗しました: $target_host"
	fi
	echo "接続先 $target_host を $target_address に解決しました。"
	backend_allow "$target_address"
	;;
*)
	target_address=$target_host
	;;
esac

# VPN装置そのものを接続先にしない。トンネルの外側と内側が同じ相手になり、
# 経路とパケットフィルタが互いを打ち消す。
if [ "${server_address:-}" = "$target_address" ]; then
	fail unknown "VPNサーバーと接続先が同じアドレスです。"
fi

# 接続先への経路は、このコンテナのトンネルの中にしか作らない。
ip route replace "$target_address/32" dev "$interface"
# トンネル以外から接続先へ出ようとする通信は拒む。トンネルが落ちているあいだ、
# 接続先への通信がDockerの通常回線へ流れることはない。
iptables -A OUTPUT -d "$target_address" ! -o "$interface" -j REJECT

mkdir -p "$socket_directory"

# トンネルの実際の様子を書き出す。engine はホスト側からこのファイルを読む。
# docker exec を呼ばずに状態を見せられるので、画面の更新が docker の応答に
# 引きずられない。秘密は書かない。
tunnel_address=$(ip -4 -o address show dev "$interface" 2>/dev/null | awk '{print $4}' | head -1)
printf '{"backend":"%s","interface":"%s","address":"%s","since":"%s","targetAddress":"%s"}\n' \
	"$backend" "$interface" "$tunnel_address" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$target_address" \
	>"$socket_directory/status.json"
chmod 644 "$socket_directory/status.json"

echo "接続先 $target_address:$target_port への中継を開始します。"
# ソケットが現れることが、トンネル・経路・フィルタまで用意できた合図である。
# engineはホスト側からこのソケットを待ち、現れたらそこへ繋ぐ。
socat \
	"UNIX-LISTEN:$socket_directory/relay.sock,fork,unlink-early,mode=0600,user=$socket_owner" \
	"TCP:$target_address:$target_port" &
relay=$!

# トンネルが落ちたら、中継を畳んでこのコンテナも終える。
#
# 中継だけが残ると、engine からは経路があるように見えたまま、繋いだ先で必ず
# 失敗する。コンテナごと終われば、次に必要になったときに engine が作り直す。
while kill -0 "$relay" 2>/dev/null; do
	if ! backend_alive; then
		fail tunnel_lost "VPNが切断されました。中継を終了します。"
	fi
	pause "$tunnel_check_seconds"
done
relay=
echo "中継が終了しました。" >&2
backend_down 2>/dev/null || true
exit 1

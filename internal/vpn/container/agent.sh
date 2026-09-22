#!/bin/sh
# ひとつのトンネルを張り、その中にある接続先への中継を差し出す。
#
# 秘密は引数にも環境変数にもイメージにも置かない。engineが docker exec の標準
# 入力で設定を書き込み、このスクリプトはそれが現れるのを待つ。読み終えた設定は
# 消す。設定はtmpfsの上にしか存在しない。
set -eu

runtime=/run/sshc-vpn
socket_directory=/run/sshc-vpn-socket
profile="$runtime/profile.json"
interface=wg0

# engineがコンテナを起動してから設定を書き込むまでの猶予。これを過ぎたら、
# 書き込む側が落ちたということなので、待ち続けずに終わる。
profile_wait_seconds=30

umask 077
mkdir -p "$runtime"

seconds=0
while [ ! -f "$profile" ]; do
	if [ "$seconds" -ge "$profile_wait_seconds" ]; then
		echo "設定を受け取れませんでした。" >&2
		exit 1
	fi
	sleep 1
	seconds=$((seconds + 1))
done

backend=$(jq -r '.backend' "$profile")
target_host=$(jq -r '.target.host' "$profile")
target_port=$(jq -r '.target.port' "$profile")
socket_owner=$(jq -r '.socketOwner' "$profile")

case "$backend" in
wireguard)
	jq -r '.wireguard.configuration' "$profile" >"$runtime/wireguard.conf"
	address=$(jq -r '.wireguard.address' "$profile")
	rm -f "$profile"
	echo "トンネルを張ります（wireguard）。"
	wireguard-go "$interface"
	wg setconf "$interface" "$runtime/wireguard.conf"
	rm -f "$runtime/wireguard.conf"
	ip address add "$address" dev "$interface"
	ip link set "$interface" up
	;;
*)
	rm -f "$profile"
	echo "未対応のbackendです: $backend" >&2
	exit 1
	;;
esac

# 接続先への経路は、このコンテナのトンネルの中にしか作らない。
ip route replace "$target_host/32" dev "$interface"
# トンネル以外から接続先へ出ようとする通信は拒む。トンネルが落ちているあいだ、
# 接続先への通信がDockerの通常回線へ流れることはない。
iptables -A OUTPUT -d "$target_host" ! -o "$interface" -j REJECT

mkdir -p "$socket_directory"
echo "接続先 $target_host:$target_port への中継を開きます。"
# ソケットが現れることが、トンネル・経路・フィルタまで用意できた合図である。
# engineはホスト側からこのソケットを待ち、現れたらそこへ繋ぐ。
exec socat \
	"UNIX-LISTEN:$socket_directory/relay.sock,fork,unlink-early,mode=0600,user=$socket_owner" \
	"TCP:$target_host:$target_port"

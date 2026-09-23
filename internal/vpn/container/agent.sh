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
# backend ごとにトンネルのI/Fが変わる。経路とパケットフィルタは、そのI/Fに
# 対して同じように作る。
interface=wg0
# connection は、strongSwan と xl2tpd がこの接続を指す名前である。
connection=sshc-vpn

# engineがコンテナを起動してから設定を書き込むまでの猶予。これを過ぎたら、
# 書き込む側が落ちたということなので、待ち続けずに終わる。
profile_wait_seconds=30

# トンネルが生きているかを見に行く間隔。短くしても落ちた瞬間が早く分かるだけで、
# 既に張られている接続は切れている。
tunnel_check_seconds=5

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
l2tp_ipsec)
	server=$(jq -r '.l2tp.server' "$profile")
	for name in ipsec.conf ipsec.secrets xl2tpd.conf ppp.options; do
		jq -r --arg name "$name" '.l2tp.documents[$name]' "$profile" >"$runtime/$name"
		chmod 600 "$runtime/$name"
	done
	rm -f "$profile"
	# 相手のアドレスはここで引く。IPsec は相手のアドレスを設定へ書くので、
	# 引く場所が違えば別の装置へ繋ぎうる。
	server_address=$(getent ahostsv4 "$server" | awk 'NR==1{print $1}')
	if [ -z "$server_address" ]; then
		echo "VPN装置の名前を引けませんでした: $server" >&2
		exit 1
	fi
	if [ "$server_address" = "$target_host" ]; then
		echo "VPN装置と接続先が同じアドレスです。" >&2
		exit 1
	fi
	sed -i "s|%SERVER_ADDRESS%|$server_address|g" "$runtime/ipsec.conf" "$runtime/xl2tpd.conf"
	ln -sf "$runtime/ipsec.conf" /etc/ipsec.conf
	ln -sf "$runtime/ipsec.secrets" /etc/ipsec.secrets

	# IPsec のポリシーが無いまま L2TP を出さない。SA が切れた瞬間に、
	# 中身が平文で出ていくことを防ぐ。
	iptables -A OUTPUT -p udp --dport 1701 -m policy --dir out --pol ipsec -j ACCEPT
	iptables -A OUTPUT -p udp --dport 1701 -j REJECT

	echo "IPsecを開始します。"
	ipsec start --nofork >"$runtime/ipsec.log" 2>&1 &
	seconds=0
	while [ ! -e /run/charon.ctl ]; do
		if [ "$seconds" -ge 15 ]; then
			echo "IPsecサービスが起動しませんでした。" >&2
			exit 1
		fi
		sleep 1
		seconds=$((seconds + 1))
	done
	if ! ipsec up "$connection" >>"$runtime/ipsec.log" 2>&1; then
		echo "IPsecが成立しませんでした。事前共有鍵・接続先・暗号方式を確認してください。" >&2
		exit 1
	fi

	echo "L2TPとPPPの認証を開始します。"
	xl2tpd -D -c "$runtime/xl2tpd.conf" -p "$runtime/xl2tpd.pid" \
		-C "$runtime/l2tp-control" >"$runtime/xl2tpd.log" 2>&1 &
	seconds=0
	while [ ! -e "$runtime/l2tp-control" ]; do
		if [ "$seconds" -ge 15 ]; then
			echo "L2TPサービスが起動しませんでした。" >&2
			exit 1
		fi
		sleep 1
		seconds=$((seconds + 1))
	done
	printf 'c %s\n' "$connection" >"$runtime/l2tp-control"
	interface=ppp0
	seconds=0
	while ! ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '; do
		if [ "$seconds" -ge 45 ]; then
			echo "PPPが成立しませんでした。利用者名とパスワードを確認してください。" >&2
			exit 1
		fi
		sleep 1
		seconds=$((seconds + 1))
	done
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
socat \
	"UNIX-LISTEN:$socket_directory/relay.sock,fork,unlink-early,mode=0600,user=$socket_owner" \
	"TCP:$target_host:$target_port" &
relay=$!

# トンネルが落ちたら、中継を畳んでこのコンテナも終える。
#
# 中継だけが残ると、engine からは経路があるように見えたまま、繋いだ先で必ず
# 失敗する。コンテナごと終われば、次に必要になったときに engine が作り直す。
while kill -0 "$relay" 2>/dev/null; do
	if ! ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '; then
		echo "トンネルが落ちました。中継を閉じます。" >&2
		kill "$relay" 2>/dev/null || true
		wait "$relay" 2>/dev/null || true
		exit 1
	fi
	sleep "$tunnel_check_seconds"
done
echo "中継が終了しました。" >&2
exit 1

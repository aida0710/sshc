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
# VPNの中で名前を引くDNSサーバー。空なら、接続先はアドレスで書かれている。
resolvers=$(jq -r 'if .dns then .dns[] else empty end' "$profile" | tr '\n' ' ')

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
		echo "VPNの中で接続先の名前を引けませんでした: $target_host" >&2
		exit 1
	fi
	echo "接続先 $target_host は $target_address でした。"
	if [ "$backend" = wireguard ]; then
		# 名前を引く前は、トンネルが運ぶのはDNSサーバーへの通信だけだった。
		# 引けた接続先をここで足す。
		allowed=""
		for resolver in $resolvers; do
			allowed="$allowed$resolver/32,"
		done
		wg set "$interface" peer "$(wg show "$interface" peers | head -1)" \
			allowed-ips "$allowed$target_address/32"
	fi
	;;
*)
	target_address=$target_host
	;;
esac

# VPN装置そのものを接続先にしない。トンネルの外側と内側が同じ相手になり、
# 経路とパケットフィルタが互いを打ち消す。
if [ "${server_address:-}" = "$target_address" ]; then
	echo "VPN装置と接続先が同じアドレスです。" >&2
	exit 1
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

echo "接続先 $target_address:$target_port への中継を開きます。"
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

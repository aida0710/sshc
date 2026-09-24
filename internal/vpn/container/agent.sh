#!/bin/sh
# ひとつのトンネルを張り、接続先への経路を作れる状態にする。
#
# 接続先ごとの中継は、engine が接続1本ごとに docker exec で connect を起動して
# 行う。ここはトンネルを張って見張り、止める合図で相手へ切断を伝える。
#
# 秘密は引数にも環境変数にもイメージにも置かない。engineが docker exec の標準
# 入力で設定を書き込み、このスクリプトはそれが現れるのを待つ。読み終えた設定は
# 消す。設定はtmpfsの上にしか存在しない。
#
# backend ごとの違いは backend-<名前>.sh に置き、ここは次の関数だけを呼ぶ。
#   backend_read   設定から自分の節を読む（このあと設定は消える）
#   backend_up     トンネルを張り、interface を決める
#   backend_ready  トンネルの相手と話せたことを確かめる
#   backend_allow  接続先を、トンネルが運ぶ相手に足す（connect が呼ぶ）
#   backend_alive  トンネルが生きているかを返す
#   backend_down   相手へ切断を伝えて畳む
set -eu

runtime=/run/sshc-vpn
# shared_directory は、ホストと共有する場所である。engine はここに現れる
# status.json と failure.json を、docker を呼ばずに読む。
shared_directory=/run/sshc-vpn-shared
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
	printf '{"reason":"%s"}\n' "$reason" >"$shared_directory/failure.json" 2>/dev/null || true
	chmod 644 "$shared_directory/failure.json" 2>/dev/null || true
	# connect が新しい接続を受けないよう、経路の控えを先に消す。
	rm -f "$runtime/route.env"
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

# timeout_seconds は、timeout に渡す残りの秒数である。GNU timeout は 0 を上限なしと
# 読むので、締め切りを過ぎていても 1 秒にする。
timeout_seconds() {
	left=$(remaining_seconds)
	if [ "$left" -lt 1 ]; then
		left=1
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

# 止める合図（docker stop）を受けたら、相手へ切断を伝えてから終わる。伝えずに
# 終わると、装置の側にセッションが残る。
shutdown() {
	trap - TERM INT
	rm -f "$runtime/route.env"
	backend_down 2>/dev/null || true
	exit 0
}
trap shutdown TERM INT

# backend を読み込む前に合図を受けても困らないよう、何もしない既定を置く。
backend_down() { :; }

seconds=0
while [ ! -f "$profile" ]; do
	if [ "$seconds" -ge "$profile_wait_seconds" ]; then
		echo "設定の受け取りがタイムアウトしました。" >&2
		exit 1
	fi
	pause 1
	seconds=$((seconds + 1))
done

backend=$(jq -r '.backend' "$profile")
# VPNの中で名前解決するDNSサーバー。空なら、接続先はアドレスでしか指定できない。
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

# connect が読む経路の控え。秘密は書かない。値はどれも engine と backend が
# 形を確かめたもの（方式の名前、interface、IPv4 アドレス）だけである。
{
	printf 'backend=%s\n' "$backend"
	printf 'interface=%s\n' "$interface"
	printf 'server_address=%s\n' "${server_address:-}"
	printf "resolvers='%s'\n" "$resolvers"
} >"$runtime/route.env"

# トンネルの実際の様子を書き出す。engine はホスト側からこのファイルを読む。
# docker exec を呼ばずに状態を見せられるので、画面の更新が docker の応答に
# 引きずられない。秘密は書かない。
#
# このファイルが現れることが、トンネルと DNS まで用意できた合図である。engine は
# これを待ってから、接続先へ繋ぎ始める。
tunnel_address=$(ip -4 -o address show dev "$interface" 2>/dev/null | awk '{print $4}' | head -1)
printf '{"backend":"%s","interface":"%s","address":"%s","since":"%s"}\n' \
	"$backend" "$interface" "$tunnel_address" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
	>"$shared_directory/status.json.pending"
chmod 644 "$shared_directory/status.json.pending"
mv "$shared_directory/status.json.pending" "$shared_directory/status.json"
echo "VPNの接続が完了しました。"

# トンネルが落ちたら、このコンテナも終える。
#
# トンネルの無い経路が残ると、engine からは経路があるように見えたまま、繋いだ先で
# 必ず失敗する。コンテナごと終われば、次に必要になったときに engine が作り直す。
while backend_alive; do
	pause "$tunnel_check_seconds"
done
fail tunnel_lost "VPNが切断されました。"

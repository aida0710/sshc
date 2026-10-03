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

# 止める合図を受けてから、相手を待っている最中のコマンドを止め、backend が相手へ切断を
# 伝え終わるまでの持ち時間。docker stop の猶予（10秒）より短くする。止める手順はどれも
# この残り（shutdown_seconds_left）の中で待つ。
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
	start_shutdown_budget
	backend_down 2>/dev/null || true
	exit 1
}

# seconds_since_boot は、カーネルが起動してからの秒数である。締め切りはこれで数える。
# 壁時計（date）は、Docker Desktop などの VM が時計を合わせ直すと飛ぶ。
seconds_since_boot() {
	read -r since_boot _ </proc/uptime
	echo "${since_boot%%.*}"
}

# remaining_seconds は、engine が決めた締め切りまでの残りの秒数である。
remaining_seconds() {
	left=$((deadline - $(seconds_since_boot)))
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

# waiting_pid は、相手を待っている最中のコマンド（wait_for_step で待っているもの）で
# ある。止める合図を受けたら、shutdown がこれを止める。
waiting_pid=

# wait_for_step は、背後で起動した、相手を待つコマンド（PID が $1）が終わるまで待ち、
# その終了コードを返す。
#
# sh は前面のコマンドが終わるまで trap を実行しない。承認待ちや応えない相手を前面で
# 待つと、止める合図を受けても、docker stop の猶予を使い切って SIGKILL で終わり、相手へ
# 切断を伝えられない。背後で起動して wait で待てば、合図を受けたときに shutdown が動く。
wait_for_step() {
	waiting_pid=$1
	step_status=0
	wait "$waiting_pid" || step_status=$?
	waiting_pid=
	return "$step_status"
}

# resolve_first_ipv4 は、$1 をこのコンテナの DNS で名前解決し、最初の IPv4 アドレスを
# resolved_address に入れる。名前解決できなければ空にする。締め切りまでしか待たない。
# コマンド置換の中で待つと止める合図が届かないので、背後で名前解決してファイルに書く。
resolve_first_ipv4() {
	resolved_address=
	timeout "$(timeout_seconds)" getent ahostsv4 -- "$1" >"$runtime/resolved" 2>/dev/null &
	if wait_for_step $!; then
		resolved_address=$(awk 'NR==1{print $1}' "$runtime/resolved")
	fi
	rm -f "$runtime/resolved"
}

# start_shutdown_budget は、止める手順の持ち時間（shutdown_seconds）を数え始める。
start_shutdown_budget() {
	shutdown_deadline=$(($(seconds_since_boot) + shutdown_seconds))
}

# shutdown_seconds_left は、止める手順の持ち時間の残りの秒数である。0 なら使い切った。
shutdown_seconds_left() {
	left=$((shutdown_deadline - $(seconds_since_boot)))
	if [ "$left" -lt 0 ]; then
		left=0
	fi
	echo "$left"
}

# shutdown_timeout_seconds は、止める手順のコマンドの timeout に渡す秒数である。GNU
# timeout は 0 を上限なしと読むので、使い切っていても 1 秒にする。
shutdown_timeout_seconds() {
	left=$(shutdown_seconds_left)
	if [ "$left" -lt 1 ]; then
		left=1
	fi
	echo "$left"
}

# step_interrupt_seconds は、相手を待っている最中のコマンドへ SIGINT を送ってから、
# SIGTERM を送るまでの長さである。openconnect は SIGINT を受けると、装置へログアウトを
# 送ってから終わる。sh は背後で起動したコマンドに SIGINT を無視させる。openconnect は
# SIGINT の扱いを自分で決めるので受け取れるが、そうしないコマンドは SIGINT では止まらず、
# SIGTERM で止まる。timeout を挟まずに起動した swanctl がそうで、uutils の timeout は
# この無視を下のコマンドへ引き継ぐので、その下の getent や ipsec up も同じになる。
step_interrupt_seconds=2

# signal_waiting_step は、相手を待っている最中のコマンドへ合図（$1）を送る。
#
# timeout は自分の process group を作り、下のコマンドもその中にいるので、group ごとに
# 送る。timeout へ送るだけでは、下のコマンドへ届くかが timeout の実装で変わる。GNU の
# timeout は受けた合図をどれも渡すが、uutils の timeout（Ubuntu 26.04 の既定）は最初の
# 合図しか渡さない。group を作らないもの（timeout を挟まずに起動した swanctl）と、まだ
# 作っていないものには、そのプロセスへ送る。
signal_waiting_step() {
	kill -s "$1" -- "-$waiting_pid" 2>/dev/null || kill -s "$1" "$waiting_pid" 2>/dev/null
}

# stop_waiting_step は、相手を待っている最中のコマンドを、止める手順の持ち時間の中で
# 止める。SIGINT、SIGTERM の順に送り、それでも終わらなければ SIGKILL で止める。
#
# 終わったコマンドは wait で片付けるまで残る（kill -0 が成功し続ける）ので、見張りの
# プロセスに合図を送らせて、こちらは wait で待つ。
stop_waiting_step() {
	if [ -z "$waiting_pid" ]; then
		return 0
	fi
	signal_waiting_step INT || true
	(
		sleep "$step_interrupt_seconds"
		signal_waiting_step TERM
		sleep "$(shutdown_seconds_left)"
		signal_waiting_step KILL
	) </dev/null >/dev/null 2>&1 &
	watchdog_pid=$!
	wait "$waiting_pid" 2>/dev/null || true
	kill "$watchdog_pid" 2>/dev/null || true
	waiting_pid=
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
	start_shutdown_budget
	rm -f "$runtime/route.env"
	stop_waiting_step
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
# 応えない相手を待つ締め切り（seconds_since_boot の秒数）。engine が待つのをやめるより
# 先に諦め、どこで止まったかをログへ残す。待つ長さは engine が決め、受け取ったときから
# 数える。
deadline=$(($(seconds_since_boot) + $(jq -r '.attemptSeconds' "$profile")))

# 読み込むのは知っている方式の手順だけにする。方式の名前は設定から読むので、そのまま
# パスに使うと、イメージの中の別のファイルを読み込みうる。
case "$backend" in
wireguard | l2tp_ipsec | openconnect | openvpn | ikev2)
	. "$backend_directory/backend-$backend.sh"
	;;
*)
	rm -f "$profile"
	echo "対応していない方式です：$backend" >&2
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
	# 応えない DNS サーバーを長く待たない。glibc の既定（1台5秒・2回）では、3台で28秒
	# かかり、connect の名前解決が engine の待つ上限を超える（connect.sh の
	# resolve_timeout_seconds）。
	printf 'options timeout:2 attempts:1\n' >>"$runtime/resolv.conf"
	# /etc/resolv.conf は Docker の bind mount である。置き換えられないので、
	# 中身だけを書き換える。
	cat "$runtime/resolv.conf" >/etc/resolv.conf
fi

# connect が読む経路の控え。秘密は書かない。値はどれも engine と backend が
# 形を確かめたもの（方式の名前、interface、IPv4 アドレス）だけである。サーバーの
# アドレスは、WireGuard の Peer が複数あれば空白で区切って並ぶので、引用符で囲む。
{
	printf 'backend=%s\n' "$backend"
	printf 'interface=%s\n' "$interface"
	printf "server_address='%s'\n" "${server_address:-}"
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

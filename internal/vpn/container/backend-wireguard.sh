# wireguard backend。agent.sh が読み込む。userspace の wireguard-go でトンネルを張る。

interface=wg0

# wireguard_stale_seconds は、最後の握手からこれを過ぎたらトンネルが死んだと
# みなす長さである。WireGuard は鍵を 2 分ごとに作り直し、180 秒（REJECT_AFTER_TIME）
# を過ぎた鍵では何も運ばない。keepalive が 25 秒なので、生きていればこの間に
# 必ず握手が起こる。
wireguard_stale_seconds=180

backend_read() {
	jq -r '.wireguard.configuration' "$profile" >"$runtime/wireguard.conf"
	wireguard_address=$(jq -r '.wireguard.address' "$profile")
}

backend_up() {
	echo "VPNに接続します（WireGuard）。"
	# wireguard-go は、カーネルが WireGuard を持っていると、カーネルの方を勧める
	# 11 行の案内を標準エラーへ出す。この経路は wireguard-go を使うと決めているので
	# 利用者には関係がなく、失敗したときに見せるログの行数を食う。出力は、起動に
	# 失敗したときだけ見せる。
	if ! wireguard-go "$interface" 2>"$runtime/wireguard-go.log"; then
		cat "$runtime/wireguard-go.log" >&2
		return 1
	fi
	wg setconf "$interface" "$runtime/wireguard.conf"
	rm -f "$runtime/wireguard.conf"
	# サーバーのアドレスは、接続先がサーバーそのものでないかを connect が確かめる
	# のに使う。名前で書いたサーバーは wg が名前解決したものになる。
	server_address=$(wg show "$interface" endpoints | awk 'NR==1 { sub(/:[0-9]+$/, "", $2); print $2 }')
	ip address add "$wireguard_address" dev "$interface"
	# interface を上げると、keepalive の設定に従って wireguard-go が相手へ
	# 握手を始める。
	ip link set "$interface" up
}

# latest_handshake は、相手との最後の握手の時刻（UNIX 秒）である。まだなら 0。
latest_handshake() {
	wg show "$interface" latest-handshakes | awk 'NR==1{print $2}'
}

# backend_ready は、相手と握手できるまで待つ。
#
# WireGuard は接続という段階を持たないので、interface が上がっただけでは、鍵や
# 相手のアドレスが正しいかは分からない。握手できて初めて、相手と話せたと言える。
backend_ready() {
	echo "ハンドシェイクの完了を待っています。"
	while [ "$(latest_handshake)" = "0" ]; do
		if [ "$(remaining_seconds)" -le 0 ]; then
			fail handshake_timeout "ハンドシェイクに失敗しました。サーバーと鍵を確認してください。"
		fi
		pause 1
	done
}

# backend_allow は、接続先を、トンネルが運ぶ相手に足す。connect が呼ぶ。
#
# 起動した時点でトンネルが運ぶのは、DNSサーバーへの通信だけである。接続先は、
# 接続に使われたものから1つずつ足す。wg set は並びを置き換えるので、いまの並びに
# 足して渡す。
backend_allow() {
	allowed=$(wg show "$interface" allowed-ips |
		awk '{ for (field = 2; field <= NF; field++) if ($field != "(none)") printf "%s,", $field }')
	wg set "$interface" peer "$(wg show "$interface" peers | head -1)" allowed-ips "$allowed$1/32"
}

# wireguard_recover_seconds は、最後のハンドシェイクが古くなってから、新しい
# ハンドシェイクを待つ長さである。スリープから戻ったマシンでは、時計だけが進み、
# ハンドシェイクは keepalive（25 秒）の次の送信で起こる。その前に切断と
# 判断しない。
wireguard_recover_seconds=40

# stale_since は、最後のハンドシェイクが古いと最初に見た時刻である。
stale_since=

backend_alive() {
	handshake=$(latest_handshake)
	now=$(date +%s)
	if [ -n "$handshake" ] && [ "$handshake" != "0" ] &&
		[ $((now - handshake)) -le "$wireguard_stale_seconds" ]; then
		stale_since=
		return 0
	fi
	if [ -z "$stale_since" ]; then
		stale_since=$now
	fi
	[ $((now - stale_since)) -lt "$wireguard_recover_seconds" ]
}

# WireGuard は状態を持たない方式なので、相手へ伝える切断は無い。
backend_down() { :; }

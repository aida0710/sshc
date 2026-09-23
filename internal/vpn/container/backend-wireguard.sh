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
	echo "トンネルを張ります（wireguard）。"
	wireguard-go "$interface"
	wg setconf "$interface" "$runtime/wireguard.conf"
	rm -f "$runtime/wireguard.conf"
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
	echo "相手との握手を待ちます。"
	while [ "$(latest_handshake)" = "0" ]; do
		if [ "$(remaining_seconds)" -le 0 ]; then
			fail handshake_timeout "WireGuardの相手と握手できませんでした。サーバー・鍵を確認してください。"
		fi
		pause 1
	done
}

# backend_allow は、名前を引いて分かった接続先を、トンネルが運ぶ相手に足す。
#
# 名前を引く前は、トンネルが運ぶのはDNSサーバーへの通信だけだった。
backend_allow() {
	allowed=""
	for resolver in $resolvers; do
		allowed="$allowed$resolver/32,"
	done
	wg set "$interface" peer "$(wg show "$interface" peers | head -1)" allowed-ips "$allowed$1/32"
}

backend_alive() {
	handshake=$(latest_handshake)
	[ -n "$handshake" ] && [ "$handshake" != "0" ] &&
		[ $(($(date +%s) - handshake)) -le "$wireguard_stale_seconds" ]
}

# WireGuard は状態を持たない方式なので、相手へ伝える切断は無い。
backend_down() { :; }

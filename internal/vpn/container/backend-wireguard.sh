# wireguard backend。agent.sh が読み込む。userspace の wireguard-go でトンネルを張る。
#
# engine は、利用者の設定ファイルを wg setconf が読む形に直して渡す（internal/vpn/wireguard.go）。
# wg setconf が読まない Address と MTU は、ここで ip に渡す。AllowedIPs の経路は作らない。
# 接続先への経路は connect が1つずつ作る。

interface=wg0

# wireguard_stale_seconds は、最後の握手からこれを過ぎたらトンネルが死んだと
# みなす長さである。WireGuard は鍵を 2 分ごとに作り直し、180 秒（REJECT_AFTER_TIME）
# を過ぎた鍵では何も運ばない。Endpoint のある Peer の keepalive は 120 秒以下（既定は
# 25 秒）なので、生きていればこの間に必ず握手が起こる。
wireguard_stale_seconds=180

backend_read() {
	jq -r '.wireguard.configuration' "$profile" >"$runtime/wireguard.conf"
	# どれも engine が形を確かめた値（IPv4 の CIDR と数）である。
	wireguard_addresses=$(jq -r '.wireguard.addresses[]' "$profile" | tr '\n' ' ')
	wireguard_mtu=$(jq -r '.wireguard.mtu // 0' "$profile")
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
	# のに使う。Peer ごとの Endpoint の IPv4 アドレスを空白で区切って並べる。名前で
	# 書いたサーバーは wg が名前解決したものになる。
	server_address=$(wg show "$interface" endpoints |
		awk '{ sub(/:[0-9]+$/, "", $2); if ($2 ~ /^[0-9.]+$/) printf "%s ", $2 }')
	for address in $wireguard_addresses; do
		ip address add "$address" dev "$interface"
	done
	if [ "$wireguard_mtu" -gt 0 ]; then
		ip link set "$interface" mtu "$wireguard_mtu"
	fi
	# interface を上げると、keepalive の設定に従って wireguard-go が相手へ
	# 握手を始める。
	ip link set "$interface" up
}

# latest_handshake は、Peer のうちいちばん新しい握手の時刻（UNIX 秒）である。まだなら 0。
latest_handshake() {
	wg show "$interface" latest-handshakes | awk '$2 > latest { latest = $2 } END { print latest + 0 }'
}

# backend_ready は、相手と握手できるまで待つ。
#
# WireGuard は接続という段階を持たないので、interface が上がっただけでは、鍵や
# 相手のアドレスが正しいかは分からない。握手できて初めて、相手と話せたと言える。
backend_ready() {
	echo "ハンドシェイクの完了を待っています。"
	while [ "$(latest_handshake)" = "0" ]; do
		if [ "$(remaining_seconds)" -le 0 ]; then
			fail handshake_timeout "ハンドシェイクに失敗しました。鍵とサーバーの指定を確認してください。"
		fi
		pause 1
	done
}

# backend_allow は、接続先が、どれかの Peer の AllowedIPs に含まれることを確かめる。
# connect が呼ぶ。
#
# WireGuard は、AllowedIPs に含まれない宛先へのパケットを、どの Peer にも送らずに捨てる。
# 経路を作っても届かないので、接続先へ繋ぎに行く前に理由を返す。
backend_allow() {
	if ! wg show "$interface" allowed-ips | awk -v target="$1" '
		function number(address,   parts) {
			split(address, parts, ".")
			return ((parts[1] * 256 + parts[2]) * 256 + parts[3]) * 256 + parts[4]
		}
		{
			for (field = 2; field <= NF; field++) {
				if ($field !~ /^[0-9.]+\/[0-9]+$/) continue
				split($field, prefix, "/")
				size = 2 ^ (32 - prefix[2])
				if (int(number(target) / size) == int(number(prefix[1]) / size)) found = 1
			}
		}
		END { exit !found }'; then
		fail target_not_allowed
	fi
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

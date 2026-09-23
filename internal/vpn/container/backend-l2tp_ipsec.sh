# l2tp_ipsec backend。agent.sh が読み込む。strongSwan、xl2tpd、pppd でトンネルを張る。

interface=ppp0

# connection は、strongSwan と xl2tpd がこの接続を指す名前である。
connection=sshc-vpn

# daemon_start_seconds は、charon と xl2tpd が制御の口を開くまで待つ上限である。
# 相手と話す前の、この機械の中だけの準備なので、締め切りとは別に数える。
daemon_start_seconds=15

backend_read() {
	server=$(jq -r '.l2tp.server' "$profile")
	for name in ipsec.conf ipsec.secrets xl2tpd.conf ppp.options; do
		jq -r --arg name "$name" '.l2tp.documents[$name]' "$profile" >"$runtime/$name"
		chmod 600 "$runtime/$name"
	done
}

# wait_for_file は、デーモンが制御の口を開くまで待つ。
wait_for_file() {
	seconds=0
	while [ ! -e "$1" ]; do
		if [ "$seconds" -ge "$daemon_start_seconds" ]; then
			return 1
		fi
		pause 1
		seconds=$((seconds + 1))
	done
}

backend_up() {
	# 相手のアドレスはここで引く。IPsec は相手のアドレスを設定へ書くので、
	# 引く場所が違えば別の装置へ繋ぎうる。
	server_address=$(getent ahostsv4 "$server" | awk 'NR==1{print $1}')
	if [ -z "$server_address" ]; then
		fail server_unresolved "VPN装置の名前を引けませんでした: $server"
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
	if ! wait_for_file /run/charon.ctl; then
		fail unknown "IPsecサービスが起動しませんでした。"
	fi
	if ! timeout "$(remaining_seconds)" ipsec up "$connection" >>"$runtime/ipsec.log" 2>&1; then
		sed -n '1,40p' "$runtime/ipsec.log" >&2
		fail ipsec_negotiation "IPsecが成立しませんでした。事前共有鍵・接続先・暗号方式を確認してください。"
	fi

	echo "L2TPとPPPの認証を開始します。"
	xl2tpd -D -c "$runtime/xl2tpd.conf" -p "$runtime/xl2tpd.pid" \
		-C "$runtime/l2tp-control" >"$runtime/xl2tpd.log" 2>&1 &
	if ! wait_for_file "$runtime/l2tp-control"; then
		fail unknown "L2TPサービスが起動しませんでした。"
	fi
	printf 'c %s\n' "$connection" >"$runtime/l2tp-control"
	if ! wait_for_address; then
		fail ppp_authentication "PPPが成立しませんでした。利用者名とパスワードを確認してください。"
	fi
}

# PPP にアドレスが付いたことが、相手の認証を通ったことを示している。
backend_ready() { :; }

backend_allow() { :; }

# pppd が lcp-echo で相手の無応答を検知すると、ppp0 のアドレスが消える。
backend_alive() {
	ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '
}

# L2TP の切断を送り、IPsec の SA を消してから止める。
backend_down() {
	if [ -e "$runtime/l2tp-control" ]; then
		printf 'd %s\n' "$connection" >"$runtime/l2tp-control" 2>/dev/null || true
	fi
	timeout "$shutdown_seconds" ipsec down "$connection" >/dev/null 2>&1 || true
	timeout "$shutdown_seconds" ipsec stop >/dev/null 2>&1 || true
}

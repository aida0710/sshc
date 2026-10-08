# l2tp_ipsec backend。agent.sh が読み込む。strongSwan、xl2tpd、pppd でトンネルを張る。

. "$backend_directory/strongswan.sh"

interface=ppp0

# connection は、strongSwan と xl2tpd がこの接続を指す名前である。
connection=sshc-vpn

# l2tp_disconnect_seconds は、L2TP の切断がサーバーへ届くのを待つ上限である。
# 止めるときの持ち時間（agent.sh の shutdown_seconds）の残りのうち、IPsec の SA を
# 消す時間を残して使う。
l2tp_disconnect_seconds=2

l2tp_diagnostics() {
	print_daemon_logs ipsec xl2tpd ppp
}

l2tp_fail() {
	# failure.jsonを公開する前に出す。engineはそのファイルを見てログを回収する。
	l2tp_diagnostics >&2
	fail "$@"
}

establish_ipsec() {
	# 応えない相手には締め切りまで IKE を送り直すので、止める合図を受けられるよう背後で
	# 待つ（wait_for_step）。ipsec up の出力は使わない。charon のログの写しで、charon が
	# 同じ行を ipsec.log へ書いている。ファイルへ書かせると書きためられ、締め切りで止めた
	# ときに失われる。
	timeout "$(timeout_seconds)" ipsec up "$connection" >/dev/null 2>&1 &
	if ! wait_for_step $!; then
		l2tp_fail ipsec_negotiation "IPsecのネゴシエーションに失敗しました。事前共有鍵、サーバー、暗号スイートを確認してください。"
	fi
	# ipsec upはQuick Modeの拒否でも終了コード0を返す。IKEだけではL2TPを
	# 運べないので、この接続のtransport modeのCHILD_SAが入ったことを確かめる。
	if ! timeout "$(timeout_seconds)" ipsec status "$connection" >"$runtime/ipsec.status" 2>&1; then
		cat "$runtime/ipsec.status" >>"$runtime/ipsec.log"
		l2tp_fail ipsec_negotiation "IPsecの接続状態を確認できませんでした。"
	fi
	cat "$runtime/ipsec.status" >>"$runtime/ipsec.log"
	if ! grep -Eq "^[[:space:]]*$connection\{[0-9]+\}:[[:space:]]+INSTALLED, TRANSPORT," "$runtime/ipsec.status"; then
		l2tp_fail ipsec_negotiation "IPsecのデータ通信用SAが確立していません。ESPの暗号スイートとIPsecのログを確認してください。"
	fi
}

ppp_authentication_failed() {
	grep -Eiq '(PAP|CHAP|MS-CHAP|EAP) authentication failed|CHAP authentication failure|EAP: peer reports authentication failure' "$runtime/ppp.log" 2>/dev/null
}

wait_for_ppp_address() {
	while ! backend_alive; do
		if ppp_authentication_failed; then
			l2tp_fail ppp_authentication "VPNサーバーがPPPの認証を拒否しました。ユーザー名、パスワード、認証方式を確認してください。"
		fi
		if ! kill -0 "$l2tp_pid" 2>/dev/null; then
			l2tp_fail unknown "L2TPサービスが終了しました。IPsec・L2TP・PPPのログを確認してください。"
		fi
		if [ "$(remaining_seconds)" -le 0 ]; then
			l2tp_fail timeout "L2TP/PPPでIPアドレスを取得できないままタイムアウトしました。認証拒否は確認できていません。"
		fi
		pause 1
	done
}

backend_read() {
	read_strongswan_documents l2tp
}

backend_up() {
	resolve_server_address "$runtime/ipsec.conf" "$runtime/xl2tpd.conf"

	# IPsec のポリシーが無いまま L2TP を出さない。SA が切れた瞬間に、
	# 中身が平文で出ていくことを防ぐ。
	iptables -A OUTPUT -p udp --dport 1701 -m policy --dir out --pol ipsec -j ACCEPT
	iptables -A OUTPUT -p udp --dport 1701 -j REJECT

	start_ipsec
	start_l2tp
	l2tp_diagnostics
}

# start_ipsec は、strongSwan を起動し、L2TP を運ぶ transport mode の SA を確立する。
start_ipsec() {
	ln -sf "$runtime/ipsec.conf" /etc/ipsec.conf
	ln -sf "$runtime/ipsec.secrets" /etc/ipsec.secrets

	echo "IPsecの接続を開始します。"
	# charon は engine が書いた strongswan.conf に従い、自分のログを ipsec.log へ足して
	# いく。starter の出力も同じファイルへ足す。
	STRONGSWAN_CONF="$runtime/strongswan.conf" ipsec start --nofork >>"$runtime/ipsec.log" 2>&1 &
	if ! wait_for_file /run/charon.ctl; then
		l2tp_fail unknown "IPsecサービスの起動に失敗しました。"
	fi
	establish_ipsec
	echo "IPsecの接続が完了しました。"
}

# start_l2tp は、IPsec の中で L2TP を接続し、PPP の認証を通ってアドレスが付くまで待つ。
start_l2tp() {
	echo "L2TPの接続とPPPの認証を開始します。"
	xl2tpd -D -c "$runtime/xl2tpd.conf" -p "$runtime/xl2tpd.pid" \
		-C "$runtime/l2tp-control" >"$runtime/xl2tpd.log" 2>&1 &
	l2tp_pid=$!
	if ! wait_for_file "$runtime/l2tp-control"; then
		l2tp_fail unknown "L2TPサービスの起動に失敗しました。"
	fi
	if ! timeout "$(timeout_seconds)" sh -c 'printf "c %s\n" "$1" >"$2"' _ "$connection" "$runtime/l2tp-control"; then
		l2tp_fail timeout "L2TPサービスへの接続要求がタイムアウトしました。"
	fi
	wait_for_ppp_address
	echo "PPPのIPアドレスを取得しました。"
}

# PPP にアドレスが付いたことが、相手の認証を通ったことを示している。
backend_ready() { :; }

backend_allow() { :; }

# pppd が lcp-echo で相手の無応答を検知すると、ppp0 のアドレスが消える。
backend_alive() {
	ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '
}

# L2TP の切断を送り、IPsec の SA を消してから止める。どの段も、止める手順の持ち時間
# （shutdown_seconds_left）の中で待つ。
backend_down() {
	if [ -e "$runtime/l2tp-control" ]; then
		# 制御の口は FIFO で、xl2tpd が止まっていると書き込みが読み手を待ち続ける。
		timeout 1 sh -c 'printf "d %s\n" "$1" >"$2"' _ "$connection" "$runtime/l2tp-control" 2>/dev/null || true
		# 切断は IPsec の中を通ってサーバーへ届く。先に IPsec を消すと、切断は
		# REJECT の規則で落ちる。PPP のアドレスが消えるまで少し待つ。IPsec の SA を消す
		# 1秒は残す。
		seconds=0
		while backend_alive && [ "$seconds" -lt "$l2tp_disconnect_seconds" ] &&
			[ "$(shutdown_seconds_left)" -gt 1 ]; do
			sleep 1
			seconds=$((seconds + 1))
		done
	fi
	timeout "$(shutdown_timeout_seconds)" ipsec down "$connection" >/dev/null 2>&1 || true
	# charon を止めるのは後始末だけなので、持ち時間が残っているときだけ行う。コンテナが
	# 終われば charon も終わる。
	if [ "$(shutdown_seconds_left)" -gt 0 ]; then
		timeout "$(shutdown_seconds_left)" ipsec stop >/dev/null 2>&1 || true
	fi
}

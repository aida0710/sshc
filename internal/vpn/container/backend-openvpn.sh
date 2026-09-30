# openvpn backend。agent.sh が読み込む。利用者の設定ファイル（.ovpn）で OpenVPN のトンネルを張る。
#
# 設定ファイルは engine が確かめてある（internal/vpn/openvpn_config.go）。コマンドを実行する
# 指示、経路や DNS を変える指示、コンテナの中に無いファイルを指す指示は含まない。そのうえで、
# sshc が決めることは設定ファイルのあとにコマンドラインで指定する。OpenVPN は、同じ指示が
# 二度あれば後のものを使う。

interface=tun0
openvpn_config="$runtime/openvpn.conf"
# openvpn_credentials は、auth-user-pass で渡すユーザー名とパスワードである。tmpfs の上に
# root だけが読める形で置く。--auth-nocache なので、OpenVPN は使うたびにここから読み直し、
# 自分のメモリには残さない。再ネゴシエーションでも使うので、経路が続くあいだは消さない。
openvpn_credentials="$runtime/openvpn.credentials"
openvpn_log="$runtime/openvpn.log"

# 失敗したとき、接続できたときに見せる OpenVPN のログの行数。docker logs の取得上限に収める。
openvpn_diagnostic_lines=40

backend_read() {
	jq -r '.openvpn.config' "$profile" >"$openvpn_config"
	servers=$(jq -r '.openvpn.servers[]' "$profile" | tr '\n' ' ')
	if [ -n "$(jq -r '.openvpn.username // ""' "$profile")" ]; then
		# 1行目がユーザー名、2行目がパスワードである。どちらも改行を含まないことを engine が
		# 確かめてある。パスワードはシェルの変数にも置かない。
		jq -r '.openvpn.username, .openvpn.password' "$profile" >"$openvpn_credentials"
	fi
}

openvpn_diagnostics() {
	tail -n "$openvpn_diagnostic_lines" "$openvpn_log" 2>/dev/null | sed 's/^/[openvpn] /'
}

openvpn_fail() {
	# failure.json を公開する前に出す。engine はそのファイルを見てログを回収する。
	openvpn_diagnostics >&2
	fail "$@"
}

openvpn_running() {
	[ -n "${openvpn_pid:-}" ] && kill -0 "$openvpn_pid" 2>/dev/null
}

# openvpn_logged は、OpenVPN のログに、正規表現に合う行があるかを返す。
openvpn_logged() {
	grep -Eq "$1" "$openvpn_log" 2>/dev/null
}

# openvpn_responded は、サーバーから応答があったかを返す。UDP でも TCP でも、サーバーが
# 最初のパケットを返すと、この行が出る。
openvpn_responded() {
	openvpn_logged 'TLS: Initial packet from|Peer Connection Initiated'
}

# openvpn_tls_failed は、TLS のハンドシェイクで、証明書の検証か鍵の確かめに失敗したかを
# 返す。繰り返しても直らないので、締め切りを待たずに断る。
openvpn_tls_failed() {
	openvpn_logged 'VERIFY [A-Z0-9 ]*ERROR|certificate verify failed|SSL routines|tls-crypt unwrapping failed|HMAC authentication failed'
}

# openvpn_fail_after_exit は、OpenVPN が接続できないまま終わった理由を返す。
openvpn_fail_after_exit() {
	if openvpn_logged 'Options error|Cannot load|Error opening configuration'; then
		openvpn_fail openvpn_configuration "OpenVPNが設定ファイルを読み込めませんでした。設定ファイルを確認してください。"
	fi
	openvpn_fail openvpn_failed "OpenVPNが終了しました。OpenVPNのログを確認してください。"
}

# openvpn_fail_after_deadline は、締め切りまでに接続できなかった理由を返す。
openvpn_fail_after_deadline() {
	if openvpn_logged 'Cannot resolve host address' && ! openvpn_logged 'link remote:'; then
		openvpn_fail server_unresolved "VPNサーバーの名前解決に失敗しました。設定ファイルのremoteを確認してください。"
	fi
	if ! openvpn_responded; then
		openvpn_fail openvpn_no_response "VPNサーバーから応答がありません。設定ファイルのremote、ネットワーク、tls-authとtls-cryptの鍵を確認してください。"
	fi
	openvpn_fail timeout "VPNサーバーとの接続がタイムアウトしました。OpenVPNのログを確認してください。"
}

# openvpn_require_server は、名前解決できるサーバーがあるかを確かめる。どれも引けなければ、
# OpenVPN を起動しても繋がらない。OpenVPN と同じく、コンテナの既定の DNS で名前解決する。
openvpn_require_server() {
	for server in $servers; do
		resolve_first_ipv4 "$server"
		if [ -n "$resolved_address" ]; then
			return 0
		fi
	done
	fail server_unresolved "VPNサーバーの名前解決に失敗しました：$servers"
}

# openvpn_wait_until_ready は、OpenVPN がトンネルを用意し終えるまで待つ。
#
# 「Initialization Sequence Completed」は、認証を通り、サーバーが配った設定を受け取り、
# interface を用意したあとに1回だけ出る。「… With Errors」で終わる行は、用意の途中で
# 失敗したことを表すので、行の終わりまで合うものだけを見る。
openvpn_wait_until_ready() {
	while ! openvpn_logged 'Initialization Sequence Completed$'; do
		if openvpn_logged 'AUTH_FAILED'; then
			openvpn_fail openvpn_authentication "VPNサーバーが認証を拒否しました。ユーザー名とパスワードを確認してください。"
		fi
		if openvpn_tls_failed; then
			openvpn_fail openvpn_tls "TLSのハンドシェイクに失敗しました。設定ファイルの証明書と鍵を確認してください。"
		fi
		if ! openvpn_running; then
			openvpn_fail_after_exit
		fi
		if [ "$(remaining_seconds)" -le 0 ]; then
			openvpn_fail_after_deadline
		fi
		pause 1
	done
}

backend_up() {
	echo "VPNに接続します（OpenVPN）。"
	openvpn_require_server
	# サーバーが配る経路と DNS は受け取らない。下の --route-noexec と --ifconfig-noexec で
	# 入れないうえに、ここでも捨てる。pull-filter は最初に合った規則を使うので、設定ファイル
	# より前に置く。「route」は route-ipv6 と route-gateway にも合う。
	set -- \
		--pull-filter ignore redirect-gateway --pull-filter ignore redirect-private \
		--pull-filter ignore route --pull-filter ignore ifconfig-ipv6 --pull-filter ignore client-nat \
		--pull-filter ignore dhcp-option --pull-filter ignore dns \
		--config "$openvpn_config"
	# interface は sshc が決める。connect は tun0 への経路とパケットフィルタを作る。DCO（カーネルが
	# データを運ぶ方式）を使うと interface の種類が変わるので使わない。
	set -- "$@" --dev "$interface" --dev-type tun --disable-dco
	# 経路と interface のアドレスは OpenVPN に作らせない。アドレスは openvpn-up が /32 で付け、
	# 接続先への経路は connect が1つずつ作る。script-security 2 は、この openvpn-up を
	# 呼ぶためだけに使う。設定ファイルのスクリプトの指示は engine が断ってある。
	set -- "$@" --route-noexec --ifconfig-noexec --script-security 2 --up "$backend_directory/openvpn-up"
	# 再接続のあいだも interface と、connect が作った経路を残す。interface が作り直された
	# ときは backend_alive が気づき、経路ごと作り直す。
	set -- "$@" --persist-tun
	# 認証に失敗したら再試行せずに終わる。ログの量は sshc が決める。5 より大きくすると、
	# パケットの中身や鍵がログに出る。
	set -- "$@" --auth-retry none --verb 3
	if [ -s "$openvpn_credentials" ]; then
		set -- "$@" --auth-user-pass "$openvpn_credentials" --auth-nocache
	fi
	openvpn "$@" >"$openvpn_log" 2>&1 &
	openvpn_pid=$!
	openvpn_wait_until_ready
	# 接続先が VPN サーバーそのものでないかを connect が確かめるのに使う。OpenVPN が実際に
	# 繋いだ相手（プロキシを使うならプロキシ）のアドレスである。
	server_address=$(sed -n 's/.* link remote: \[AF_INET\]\([0-9.]*\):[0-9]*.*/\1/p' "$openvpn_log" | tail -n 1)
	if ! wait_for_address; then
		openvpn_fail openvpn_failed "VPNサーバーからIPv4アドレスを受け取れませんでした。"
	fi
	openvpn_interface_index=$(cat "/sys/class/net/$interface/ifindex")
	echo "OpenVPNの接続が完了しました。"
	openvpn_diagnostics
}

# 「Initialization Sequence Completed」が、認証を通ったことを示している。
backend_ready() { :; }

backend_allow() { :; }

# OpenVPN が終わるか、interface が作り直されるか、アドレスが消えたら、トンネルは運べない。
# 作り直された interface には、connect が作った経路が無い。
backend_alive() {
	openvpn_running &&
		[ "$(cat "/sys/class/net/$interface/ifindex" 2>/dev/null)" = "$openvpn_interface_index" ] &&
		ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '
}

# SIGTERM を受けた OpenVPN は、設定ファイルに explicit-exit-notify があればサーバーへ切断を
# 伝えてから終わる。終わった OpenVPN は pause の wait が片付けるので、sleep ではなく pause で
# 待つ。
backend_down() {
	if ! openvpn_running; then
		return 0
	fi
	kill -TERM "$openvpn_pid" 2>/dev/null || true
	while openvpn_running && [ "$(shutdown_seconds_left)" -gt 0 ]; do
		pause 1
	done
}

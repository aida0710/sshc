# openconnect backend。agent.sh が読み込む。AnyConnect 系の装置へ openconnect で繋ぐ。

interface=vpn0
openconnect_pid_file="$runtime/openconnect.pid"

backend_read() {
	server=$(jq -r '.openconnect.server' "$profile")
	username=$(jq -r '.openconnect.username' "$profile")
	protocol=$(jq -r '.openconnect.protocol' "$profile")
	certificate=$(jq -r '.openconnect.serverCertificate' "$profile")
	# パスワードは変数にだけ置き、引数にも環境変数にも渡さない。openconnect へは
	# 標準入力で渡す。
	password=$(jq -r '.openconnect.password' "$profile")
	# 装置がもう一問聞いてきたときに送る1行。openconnect は、パスワードの次の
	# 質問にも標準入力の次の行を使う。中身が何かは engine が決めてある。
	second_factor=$(jq -r '.openconnect.secondFactor // ""' "$profile")
	waits_for_approval=$(jq -r '.openconnect.waitsForApproval // false' "$profile")
}

# 答えは、聞かれるぶんだけ渡す。二段目が無いのに空行を渡すと、二段目を聞く
# 装置に対して「空の答え」を送ってしまい、失敗の理由が分からなくなる。
send_answers() {
	printf '%s\n' "$password"
	if [ -n "$second_factor" ]; then
		printf '%s\n' "$second_factor"
	fi
}

backend_up() {
	echo "VPNに接続します（OpenConnect）。"
	set -- --protocol="$protocol" --user="$username" --interface="$interface" \
		--script="$backend_directory/vpnc-script" --passwd-on-stdin --non-inter --background \
		--pid-file="$openconnect_pid_file"
	if [ -n "$certificate" ]; then
		set -- "$@" --servercert="$certificate"
	fi
	if [ -n "$second_factor" ]; then
		echo "二要素認証の入力を送ります。"
	fi
	if [ "$waits_for_approval" = "true" ]; then
		echo "スマートフォンでの承認を待っています。承認するまでサーバーは応答しません。"
	fi
	# --background は、繋がったあとに自分を背後へ回す。ここが 0 で返らなければ
	# 繋がっていない。
	if ! send_answers | timeout "$(remaining_seconds)" openconnect "$@" "$server" \
		>"$runtime/openconnect.log" 2>&1; then
		password=
		second_factor=
		sed -n '1,40p' "$runtime/openconnect.log" >&2
		fail openconnect_failed "VPNサーバーへの接続に失敗しました。ユーザー名、パスワード、二要素認証、プロトコル、証明書を確認してください。"
	fi
	password=
	second_factor=
	if ! wait_for_address; then
		sed -n '1,40p' "$runtime/openconnect.log" >&2
		fail openconnect_failed "VPNサーバーからトンネルのアドレスを取得するのに失敗しました。"
	fi
}

# 装置がアドレスを配ったことが、認証を通ったことを示している。
backend_ready() { :; }

backend_allow() { :; }

openconnect_running() {
	[ -s "$openconnect_pid_file" ] && kill -0 "$(cat "$openconnect_pid_file")" 2>/dev/null
}

# openconnect は、相手とのやり取りを諦めると終わる。終わっていなくても、
# interface のアドレスが消えていれば運べない。
backend_alive() {
	openconnect_running && ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '
}

# openconnect は SIGINT を受けると、装置へ logout を送ってから終わる。送らずに
# 終わると、装置の側にセッションが残り、同時接続の枠を使い続ける。
backend_down() {
	if ! openconnect_running; then
		return 0
	fi
	kill -INT "$(cat "$openconnect_pid_file")" 2>/dev/null || true
	seconds=0
	while openconnect_running && [ "$seconds" -lt "$shutdown_seconds" ]; do
		sleep 1
		seconds=$((seconds + 1))
	done
}

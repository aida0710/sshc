# strongSwan を使う backend（l2tp_ipsec と ikev2）が共通に使う手順。backend の
# 手順が読み込む。
#
# engine は strongSwan などが読む本文を組み立てて渡し、ここはそれを置く。VPN
# サーバーのアドレスだけは、コンテナの中で名前解決して本文へ埋める。IPsec は相手の
# アドレスを設定に書くので、名前解決する場所が違えば別のサーバーへ接続しうる。

# daemon_start_seconds は、デーモンが制御の口を開くまで待つ上限である。
# 相手と話す前の、このマシンの中だけの準備なので、締め切りとは別に数える。
daemon_start_seconds=15

# デーモンの通常ログだけを残す。PPPのdebugは認証パケットも出すため有効にしない。
# L2TP/IPsec の3つのログがdocker logsの取得上限に収まる行数にする。
diagnostic_tail_lines=60

# read_strongswan_documents は、設定の <節>.documents の本文を、それぞれの名前で
# runtime へ置き、VPN サーバーの名前を server に読む。
read_strongswan_documents() {
	section=$1
	server=$(jq -r --arg section "$section" '.[$section].server' "$profile")
	for name in $(jq -r --arg section "$section" '.[$section].documents | keys[]' "$profile"); do
		# 名前は engine が決めたものだけだが、runtime の外へ書かないよう確かめる。
		case "$name" in
		.* | *[!A-Za-z0-9._-]*)
			echo "設定のファイル名が正しくありません: $name" >&2
			return 1
			;;
		esac
		jq -r --arg section "$section" --arg name "$name" '.[$section].documents[$name]' "$profile" >"$runtime/$name"
		chmod 600 "$runtime/$name"
	done
}

# resolve_server_address は、VPN サーバーを名前解決し、引数の本文の中の印を、
# そのアドレスで置き換える。アドレスは server_address に残す。connect は、接続先が
# サーバーそのものでないかを、このアドレスで確かめる。
resolve_server_address() {
	server_address=$(getent ahostsv4 "$server" | awk 'NR==1{print $1}')
	if [ -z "$server_address" ]; then
		fail server_unresolved "VPNサーバーの名前解決に失敗しました: $server"
	fi
	for document in "$@"; do
		sed -i "s|%SERVER_ADDRESS%|$server_address|g" "$document"
	done
	printf 'VPNサーバーの名前解決: %s → %s\n' "$server" "$server_address"
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

# print_daemon_logs は、デーモンのログの末尾を、名前を前に付けて出す。引数は、
# runtime の <名前>.log の名前である。
print_daemon_logs() {
	for log_name in "$@"; do
		if [ -s "$runtime/$log_name.log" ]; then
			tail -n "$diagnostic_tail_lines" "$runtime/$log_name.log" | sed "s/^/[$log_name] /"
		else
			printf '[%s] ログはまだありません。\n' "$log_name"
		fi
	done
}

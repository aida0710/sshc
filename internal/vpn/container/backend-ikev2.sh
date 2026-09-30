# ikev2 backend。agent.sh が読み込む。strongSwan の charon を swanctl で操作し、
# IKEv2/IPsec でトンネルを張る。
#
# トンネルは XFRM インターフェースに結び付ける（route-based）。IPsec のポリシーは
# このインターフェースを通る通信にしか効かないので、サーバーが 0.0.0.0/0 を配っても
# コンテナの既定経路は変わらない。インターフェースへの経路が無い通信はトンネルに
# 入らず、トンネルが無いあいだにこのインターフェースへ向かった通信は捨てられる。
# 接続先への経路は connect が1つずつ作る。

. "$backend_directory/strongswan.sh"

# connection は、swanctl.conf の中で、この接続と CHILD_SA を指す名前である。
connection=sshc-vpn

# swanctl_directory は、swanctl.conf を置く場所である。swanctl は、証明書を
# swanctl.conf と同じ場所の x509ca などから読む。公的な認証局で確かめるときは、
# その x509ca へ ca-certificates の証明書を並べる。
swanctl_directory="$runtime/swanctl"

# swanctl_credential_directories は、swanctl が証明書と鍵を探すディレクトリである。
# 無いと、読み込むたびに1つずつ警告をログへ書く。
swanctl_credential_directories="x509 x509ca x509ocsp x509aa x509ac x509crl pubkey private rsa ecdsa bliss pkcs8 pkcs12"

# vici_socket は、charon が swanctl の指示を受け付ける口である。
vici_socket=/var/run/charon.vici

# charon_log は、charon のログである（engine が strongswan.conf に書いた場所）。
# 接続できなかった理由は、ここから読む。
charon_log="$runtime/charon.log"

backend_read() {
	read_strongswan_documents ikev2
	interface=$(jq -r '.ikev2.link.name' "$profile")
	interface_id=$(jq -r '.ikev2.link.id' "$profile")
	interface_mtu=$(jq -r '.ikev2.link.mtu' "$profile")
	public_authorities=$(jq -r '.ikev2.publicAuthorities // false' "$profile")
}

# swanctl_run は、swanctl を実行し、出力を swanctl.log へ足す。
#
# swanctl は、イメージに入っていないプラグインを読もうとして1つずつ警告を書き、
# 公的な認証局の証明書を1枚ずつ読んだと書く。どちらも接続の成否に関係なく、失敗した
# ときに見せるログの行数を食うので、swanctl.log からは除く。
#
# 応えない相手には --timeout まで IKE を送り直すので、止める合図を受けられるよう背後で
# 待つ（wait_for_step）。
swanctl_run() {
	status=0
	swanctl "$@" >"$runtime/swanctl.out" 2>&1 &
	wait_for_step $! || status=$?
	grep -v -e "^plugin '[^']*': failed to load" -e "^loaded certificate from '$swanctl_directory/x509ca/" \
		"$runtime/swanctl.out" >>"$runtime/swanctl.log" || true
	return "$status"
}

ikev2_fail() {
	# failure.jsonを公開する前に出す。engineはそのファイルを見てログを回収する。
	print_daemon_logs charon swanctl >&2
	fail "$@"
}

# ikev2_failure_reason は、charon のログから、接続できなかった理由の語を選ぶ。
#
# 先に当たったものを使う。認証やサーバーの検証に失敗する前に再送していても、
# 失敗の理由は認証や検証の方である。
ikev2_failure_reason() {
	if grep -q 'received NO_PROPOSAL_CHOSEN notify' "$charon_log" 2>/dev/null; then
		echo ike_proposal_mismatch
	elif grep -Eq 'no trusted [A-Z]+ public key found|constraint check failed|MAC mismatched' "$charon_log" 2>/dev/null; then
		echo ike_server_unverified
	elif grep -Eq 'received AUTHENTICATION_FAILED notify|EAP_[A-Z0-9]+ method failed|received EAP_FAILURE' "$charon_log" 2>/dev/null; then
		echo ike_authentication
	elif grep -Eq 'retransmit [0-9]+ of request|giving up after [0-9]+ retransmits' "$charon_log" 2>/dev/null; then
		echo ike_no_response
	else
		echo ipsec_negotiation
	fi
}

# ikev2_failure_sentence は、理由の語に添えて、コンテナのログへ書く文である。
ikev2_failure_sentence() {
	case "$1" in
	ike_proposal_mismatch) echo "暗号スイートのネゴシエーションに失敗しました。IKEとESPの暗号スイートを確認してください。" ;;
	ike_server_unverified) echo "VPNサーバーの証明書を検証できませんでした。サーバーのIDとCAの証明書を確認してください。" ;;
	ike_authentication) echo "IKEv2の認証に失敗しました。ユーザー名、パスワード、事前共有鍵、IDを確認してください。" ;;
	ike_no_response) echo "VPNサーバーから応答がありません。サーバーの指定と、UDPの500番と4500番でVPNサーバーに到達できるかを確認してください。" ;;
	*) echo "IPsecのネゴシエーションに失敗しました。IPsecのログを確認してください。" ;;
	esac
}

# ikev2_child_installed は、この接続の CHILD_SA がカーネルに入っているかを返す。
#
# IKE_SA だけが確立し、CHILD_SA がサーバーに断られることがある（ESP の暗号スイートが
# 合わない場合など）。IKE_SA だけではデータを運べない。
ikev2_child_installed() {
	swanctl --list-sas --ike "$connection" 2>/dev/null |
		grep -Eq "^[[:space:]]+$connection: #[0-9]+, reqid [0-9]+, INSTALLED, TUNNEL"
}

establish_ikev2() {
	if ! swanctl_run --load-all --file "$swanctl_directory/swanctl.conf"; then
		ikev2_fail unknown "IPsecの設定を読み込めませんでした。"
	fi
	# パスワードと事前共有鍵は charon が読み込んだ。ファイルには残さない。
	rm -f "$swanctl_directory/swanctl.conf"
	if [ "$public_authorities" = true ]; then
		printf '公的な認証局の証明書を%s件読み込みました。\n' \
			"$(grep -c "^loaded certificate from '$swanctl_directory/x509ca/" "$runtime/swanctl.out" || true)"
	fi
	# charon のログは charon.log にあるので、swanctl には重ねて出させない。
	if ! swanctl_run --initiate --child "$connection" --timeout "$(timeout_seconds)" --loglevel -1; then
		reason=$(ikev2_failure_reason)
		ikev2_fail "$reason" "$(ikev2_failure_sentence "$reason")"
	fi
	if ! ikev2_child_installed; then
		ikev2_fail ipsec_negotiation "IPsecのデータ通信用SAが確立していません。IPsecのログを確認してください。"
	fi
}

backend_up() {
	echo "VPNに接続します（IKEv2/IPsec）。"
	resolve_server_address "$runtime/swanctl.conf"
	for directory in $swanctl_credential_directories; do
		mkdir -p "$swanctl_directory/$directory"
	done
	mv "$runtime/swanctl.conf" "$swanctl_directory/swanctl.conf"
	if [ "$public_authorities" = true ]; then
		# swanctl は1つのファイルから証明書を1枚しか読まない。ca-certificates が
		# 1枚ずつ置いたファイルを並べる。
		ln -s /etc/ssl/certs/*.pem "$swanctl_directory/x509ca/"
	fi
	# charon が仮想 IP を付ける先なので、charon より先に作る。
	if ! ip link add "$interface" type xfrm if_id "$interface_id" 2>"$runtime/xfrm.log"; then
		cat "$runtime/xfrm.log" >&2
		fail xfrm_interface_unavailable "IPsecのXFRMインターフェースを作成できませんでした。DockerのLinuxカーネルが対応していない可能性があります。"
	fi
	ip link set "$interface" mtu "$interface_mtu" up

	echo "IPsecの接続を開始します。"
	STRONGSWAN_CONF="$runtime/strongswan.conf" /usr/lib/ipsec/charon >>"$charon_log" 2>&1 &
	charon_pid=$!
	if ! wait_for_file "$vici_socket"; then
		ikev2_fail unknown "IPsecサービスの起動に失敗しました。"
	fi
	establish_ikev2
	if ! wait_for_address; then
		ikev2_fail ipsec_negotiation "VPNサーバーから仮想IPアドレスを取得できませんでした。"
	fi
	echo "IPsecの接続が完了しました。"
	print_daemon_logs charon
}

# CHILD_SA が入ったことが、サーバーの検証とこちらの認証を通ったことを示している。
backend_ready() { :; }

# サーバーが許した範囲（トラフィックセレクター）は起動した時点で決まっている。
# 接続先ごとに足すものは無い。
backend_allow() { :; }

# DPD でサーバーの無応答を検知すると、charon は SA と仮想 IP を消す。
backend_alive() {
	kill -0 "$charon_pid" 2>/dev/null && ikev2_child_installed &&
		ip -4 address show dev "$interface" 2>/dev/null | grep -q 'inet '
}

# サーバーへ IKE_SA の削除を伝えてから charon を止める。伝えずに終わると、サーバーの
# 側にセッションと仮想 IP の割り当てが残る。
backend_down() {
	if [ -S "$vici_socket" ]; then
		timeout "$(shutdown_timeout_seconds)" swanctl --terminate --ike "$connection" \
			--timeout "$(shutdown_timeout_seconds)" --loglevel -1 >/dev/null 2>&1 || true
	fi
	if [ -n "${charon_pid:-}" ]; then
		kill "$charon_pid" 2>/dev/null || true
	fi
}

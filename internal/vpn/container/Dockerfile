# sshc のVPNコンテナ。依存物だけを含み、sshc由来のものは何も焼かない。
#
# 焼くのは agent.sh と backend ごとの手順（strongSwan を使う backend が共通に使う
# strongswan.sh を含む）と、接続1本ぶんの中継（connect）と、openconnect が呼ぶ
# vpnc-script と、OpenVPN が呼ぶ openvpn-up だけである。どれも秘密を持たない。設定と秘密は、engineが起動後に標準入力で渡し、tmpfsの上にしか
# 置かない。
# 基のイメージは digest で固定する。イメージのタグは agent.sh と Dockerfile の
# 内容から作るので、tag を追いかけると、同じタグのまま中身の違うイメージが機械
# ごとにできる。上げるときは digest を書き換える。
FROM ubuntu:24.04@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517

# パッケージのバージョンも固定する。base の digest だけを固定しても、apt はその日の
# 最新を入れるので、同じタグのままマシンごとに中身が変わる。Ubuntu の snapshot の
# 時刻で取る。上げるときに一緒に書き換えるものは、下の InRelease の説明に並べる。
#
# 取得先は snapshot だけにする。APT::Snapshot を使うと、apt は snapshot に加えて
# いつものミラー（archive、security、ports）の索引も取りに行き、ミラーが同期して
# いる最中だと、イメージの作成ごと失敗する。arm64 の ports.ubuntu.com は、apt が
# snapshot の場所を知らず、どのパッケージも見つからなかった。snapshot.ubuntu.com
# の ubuntu は amd64 と arm64 の索引とパッケージを、noble-security も含めて配って
# いるので、どちらのアーキテクチャも同じ sources で取れる。
#
# base のイメージには CA 証明書が無く、snapshot は HTTPS でしか配られていない。
# そこで取得のときだけ TLS の相手確認を外し、索引の出発点である suite ごとの
# InRelease を SHA-256 で固定する。apt の署名の確かめだけでは足りない。署名が示すのは
# 「Ubuntu がいつか署名した索引」であることだけで、指定した時刻の索引であることは
# 示さない。相手確認を外した通り道では、別の時刻（脆弱性が直る前）の署名済みの索引を
# 返されても apt は受け入れる。InRelease を固定すれば、索引は InRelease のハッシュで、
# パッケージは索引のハッシュで確かめられるので、通り道の相手が誰であっても、
# 固定した時刻と違う中身は入らない。
#
# snapshot の時刻を上げるときは、同じ変更で次の値を書き換える。
#   - base の digest（上の FROM）
#   - snapshot（下の RUN の先頭）
#   - InRelease の SHA-256（suite ごとに1つ）。
#     curl -fsSO https://snapshot.ubuntu.com/ubuntu/<時刻>/dists/<suite>/InRelease で
#     取り、gpgv --keyring /usr/share/keyrings/ubuntu-archive-keyring.gpg InRelease で
#     署名を確かめてから、sha256sum InRelease の値を書く。
# 上げる時機は docs/releasing.md に書いてある。
#
# IKEv2 のために、次のパッケージを入れる。
#   strongswan-swanctl: 接続を XFRM インターフェースに結び付ける（stroke の
#     ipsec.conf は if_id を扱えない）。
#   libcharon-extauth-plugins: EAP-MSCHAPv2。
#   libstrongswan-standard-plugins: EAP-MSCHAPv2 が使う MD4 と DES、ECDSA の証明書、
#     AES-GCM（openssl など）。
#   ca-certificates: VPN サーバーの証明書を公的な認証局で確かめる。apt と同じ手順の
#     中で入るので、上の apt の取得の相手確認には使えない。
# snapshot.ubuntu.com は、ときどき数分のあいだ 503 を返したり、とても遅くなったりする。
# apt は一時的な失敗を既定で3回（待ちは最長30秒）まで試し直すが、それでは足りずに
# イメージの作成ごと失敗したことがある。8回・最長60秒（1つのファイルで最長3分ほど）
# まで待つ。
RUN snapshot=20260920T000000Z \
    && printf '%s\n' \
       'Types: deb' \
       "URIs: https://snapshot.ubuntu.com/ubuntu/$snapshot/" \
       'Suites: noble noble-updates noble-backports noble-security' \
       'Components: main universe restricted multiverse' \
       'Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg' \
       > /etc/apt/sources.list.d/ubuntu.sources \
    && printf '%s\n' 'Acquire::Retries "8";' 'Acquire::Retries::Delay::Maximum "60";' \
       > /etc/apt/apt.conf.d/80sshc-retries \
    && apt-get -o Acquire::https::Verify-Peer=false update \
    && lists="/var/lib/apt/lists/snapshot.ubuntu.com_ubuntu_${snapshot}_dists" \
    && printf '%s\n' \
       "cdb2f31d809f589719a53c6ad15f255b27569c4059542ada282aaa21b8e164b0  ${lists}_noble_InRelease" \
       "aeaf85abd41363b42bf9951020685e0f28971d0626afd28f210919a2b8f89810  ${lists}_noble-updates_InRelease" \
       "9cccfba9644babbf5805575f513380ecc972708661c2cfa6783bfea877c1d11a  ${lists}_noble-backports_InRelease" \
       "603d902fbcedd1666b0897c005771162a7b36288a56aac6a5f557556d53d66de  ${lists}_noble-security_InRelease" \
       | sha256sum --check --strict - \
    && DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::https::Verify-Peer=false \
       install -y --no-install-recommends \
       iproute2 iptables wireguard-tools wireguard-go socat jq \
       strongswan strongswan-starter xl2tpd ppp \
       openconnect \
       openvpn \
       strongswan-swanctl libcharon-extauth-plugins libstrongswan-standard-plugins ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY agent.sh /usr/local/bin/sshc-vpn-agent
# backend ごとの手順、接続1本ぶんの中継、openconnect と OpenVPN が呼ぶ script。どれも
# 秘密を持たず、プロファイルごとにも変わらない。設定を置く tmpfs は noexec なので、
# 実行するものはイメージ側に置く。
COPY backend-wireguard.sh backend-l2tp_ipsec.sh backend-openconnect.sh backend-openvpn.sh backend-ikev2.sh \
     strongswan.sh vpnc-script openvpn-up /usr/local/lib/sshc-vpn/
COPY connect.sh /usr/local/lib/sshc-vpn/connect
RUN chmod 0755 /usr/local/bin/sshc-vpn-agent /usr/local/lib/sshc-vpn/vpnc-script /usr/local/lib/sshc-vpn/openvpn-up \
       /usr/local/lib/sshc-vpn/connect \
    && chmod 0644 /usr/local/lib/sshc-vpn/backend-*.sh /usr/local/lib/sshc-vpn/strongswan.sh

ENTRYPOINT ["/usr/local/bin/sshc-vpn-agent"]

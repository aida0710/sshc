# sshc のVPNコンテナ。依存物だけを含み、sshc由来のものは何も焼かない。
#
# 焼くのは agent.sh と backend ごとの手順と、接続1本ぶんの中継（connect）と、
# openconnect が呼ぶ vpnc-script だけである。どれも秘密を持たない。設定と秘密は、engineが起動後に標準入力で渡し、tmpfsの上にしか
# 置かない。
# 基のイメージは digest で固定する。イメージのタグは agent.sh と Dockerfile の
# 内容から作るので、tag を追いかけると、同じタグのまま中身の違うイメージが機械
# ごとにできる。上げるときは digest を書き換える。
FROM ubuntu:24.04@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517

# パッケージの版も固定する。base の digest だけを固定しても、apt はその日の最新を
# 入れるので、同じタグのまま機械ごとに中身が変わる。Ubuntu の snapshot の時刻で
# 取る。上げるときは、base の digest と一緒にこの時刻を書き換える。
#
# arm64 のイメージは、パッケージを ports.ubuntu.com から取る。apt が snapshot の
# 場所を知っているのは archive.ubuntu.com と security.ubuntu.com だけで、
# ports.ubuntu.com のままでは、索引は取れるのにどのパッケージも見つからない。
# snapshot.ubuntu.com の ubuntu は arm64 の索引とパッケージも配っているので、
# ports.ubuntu.com の snapshot としてその場所を明示する。
#
# base のイメージには CA 証明書が無く、snapshot は HTTPS でしか配られていない。
# そこで取得のときだけ TLS の相手確認を外す。apt は受け取った索引を Ubuntu の
# archive 鍵の署名で確かめ、パッケージをその索引のハッシュで確かめるので、
# 通り道の相手が誰であっても、差し替えられた中身は入らない。
RUN echo 'APT::Snapshot "20260920T000000Z";' > /etc/apt/apt.conf.d/50sshc-snapshot \
    && echo 'Acquire::Snapshots::URI::Host::ports.ubuntu.com "https://snapshot.ubuntu.com/ubuntu/@SNAPSHOTID@/";' \
       >> /etc/apt/apt.conf.d/50sshc-snapshot \
    && apt-get -o Acquire::https::Verify-Peer=false update \
    && DEBIAN_FRONTEND=noninteractive apt-get -o Acquire::https::Verify-Peer=false \
       install -y --no-install-recommends \
       iproute2 iptables wireguard-tools wireguard-go socat jq \
       strongswan strongswan-starter xl2tpd ppp \
       openconnect \
    && rm -rf /var/lib/apt/lists/*

COPY agent.sh /usr/local/bin/sshc-vpn-agent
# backend ごとの手順、接続1本ぶんの中継、openconnect が呼ぶ script。どれも秘密を
# 持たず、プロファイルごとにも変わらない。設定を置く tmpfs は noexec なので、
# 実行するものはイメージ側に置く。
COPY backend-wireguard.sh backend-l2tp_ipsec.sh backend-openconnect.sh vpnc-script /usr/local/lib/sshc-vpn/
COPY connect.sh /usr/local/lib/sshc-vpn/connect
RUN chmod 0755 /usr/local/bin/sshc-vpn-agent /usr/local/lib/sshc-vpn/vpnc-script /usr/local/lib/sshc-vpn/connect \
    && chmod 0644 /usr/local/lib/sshc-vpn/backend-*.sh

ENTRYPOINT ["/usr/local/bin/sshc-vpn-agent"]

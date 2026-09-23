# sshc のVPNコンテナ。依存物だけを含み、sshc由来のものは何も焼かない。
#
# 焼くのは agent.sh と、openconnect が呼ぶ vpnc-script だけである。どちらも
# 秘密を持たない。設定と秘密は、engineが起動後に標準入力で渡し、tmpfsの上にしか
# 置かない。
# 基のイメージは digest で固定する。イメージのタグは agent.sh と Dockerfile の
# 内容から作るので、tag を追いかけると、同じタグのまま中身の違うイメージが機械
# ごとにできる。上げるときは digest を書き換える。
FROM ubuntu:24.04@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       iproute2 iptables wireguard-tools wireguard-go socat jq \
       strongswan strongswan-starter xl2tpd ppp \
       openconnect \
    && rm -rf /var/lib/apt/lists/*

COPY agent.sh /usr/local/bin/sshc-vpn-agent
# openconnect が呼ぶ script は、秘密を持たず、プロファイルごとにも変わらない。
# 設定を置く tmpfs は noexec なので、実行するものはイメージ側に置く。
COPY vpnc-script /usr/local/lib/sshc-vpn/vpnc-script
RUN chmod 0755 /usr/local/bin/sshc-vpn-agent /usr/local/lib/sshc-vpn/vpnc-script

ENTRYPOINT ["/usr/local/bin/sshc-vpn-agent"]

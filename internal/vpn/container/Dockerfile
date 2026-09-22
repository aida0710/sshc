# sshc のVPNコンテナ。依存物だけを含み、sshc由来のものは何も焼かない。
#
# agent.sh はこのイメージの唯一の中身である。設定と秘密は、engineが起動後に
# 標準入力で渡し、tmpfsの上にしか置かない。
FROM ubuntu:24.04

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
       iproute2 iptables wireguard-tools wireguard-go socat jq \
    && rm -rf /var/lib/apt/lists/*

COPY agent.sh /usr/local/bin/sshc-vpn-agent
RUN chmod 0755 /usr/local/bin/sshc-vpn-agent

ENTRYPOINT ["/usr/local/bin/sshc-vpn-agent"]

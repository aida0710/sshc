FROM alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8 AS builder
RUN apk add --no-cache build-base linux-headers
COPY seed-random.c /seed-random.c
RUN cc -Os -s -static -Wall -Wextra -Werror /seed-random.c -o /seed-random

FROM alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8
COPY --from=builder /seed-random /usr/local/bin/seed-random

RUN apk add --no-cache openssh-server openssh-client bash ca-certificates \
    && ssh-keygen -t ed25519 -N '' -f /etc/ssh/ssh_host_ed25519_key \
    && mkdir -p /root/.ssh /run/sshd /demo \
    && ssh-keygen -t ed25519 -N '' -f /root/.ssh/id_ed25519 \
    && cp /root/.ssh/id_ed25519.pub /root/.ssh/authorized_keys \
    && chmod 700 /root/.ssh \
    && chmod 600 /root/.ssh/authorized_keys \
    && sed -i 's/^root:[^:]*:/root::/' /etc/shadow \
    && printf 'PermitRootLogin yes\nPasswordAuthentication no\nAllowTcpForwarding yes\nSubsystem sftp internal-sftp\n' > /etc/ssh/sshd_config \
    && printf 'Host demo-a\n  HostName 10.77.0.11\n  User root\nHost demo-b\n  HostName 10.77.0.12\n  User root\nHost *\n  IdentityFile ~/.ssh/id_ed25519\n  StrictHostKeyChecking yes\n' > /root/.ssh/config \
    && for address in 10.77.0.11 10.77.0.12; do \
         printf '%s ' "$address" >> /root/.ssh/known_hosts; \
         cut -d ' ' -f 1,2 /etc/ssh/ssh_host_ed25519_key.pub >> /root/.ssh/known_hosts; \
       done \
    && chmod 600 /root/.ssh/config /root/.ssh/known_hosts \
    && printf 'Welcome to the sshc demo.\nEdit this file using SFTP, or connect from the CLI.\n' > /root/welcome.txt

COPY init /init
RUN chmod 755 /init

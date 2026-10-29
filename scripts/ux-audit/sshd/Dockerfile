# 撮影用の使い捨て sshd。実在のデータは入れず、架空のファイルだけを置く。
FROM python:3.14.8-alpine3.24
RUN apk add --no-cache openssh-server openssh-sftp-server bash imagemagick ffmpeg font-dejavu fontconfig \
 && ssh-keygen -A \
 && adduser -D -s /bin/bash deploy \
 && passwd -u deploy \
 && mkdir -p /home/deploy/.ssh && chmod 700 /home/deploy/.ssh
COPY populate.sh /populate.sh
RUN sh /populate.sh && chown -R deploy:deploy /home/deploy
RUN printf 'PasswordAuthentication no\nPermitRootLogin no\nSubsystem sftp internal-sftp\n' > /etc/ssh/sshd_config.d/10-fixture.conf 2>/dev/null || printf 'PasswordAuthentication no\nPermitRootLogin no\n' >> /etc/ssh/sshd_config
EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]

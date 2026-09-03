FROM ubuntu@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-server libpam-google-authenticator \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --create-home --shell /bin/bash tester \
    && printf '%s\n' 'tester:integration-totp-password' | chpasswd \
    && install -d -m 0755 /run/sshd \
    && ssh-keygen -A

RUN printf '%s\n' \
      'JBSWY3DPEHPK3PXP' \
      '" TOTP_AUTH' \
      '" WINDOW_SIZE 3' \
      '" RATE_LIMIT 20 30' \
      > /home/tester/.google_authenticator \
    && chown tester:tester /home/tester/.google_authenticator \
    && chmod 0600 /home/tester/.google_authenticator \
    && printf '%s\n' \
      'auth required pam_unix.so' \
      'auth required pam_google_authenticator.so' \
      'account required pam_unix.so' \
      'session required pam_permit.so' \
      > /etc/pam.d/sshd \
    && printf '%s\n' \
      'Port 2222' \
      'ListenAddress 0.0.0.0' \
      'UsePAM yes' \
      'KbdInteractiveAuthentication yes' \
      'PasswordAuthentication no' \
      'PubkeyAuthentication no' \
      'AuthenticationMethods keyboard-interactive' \
      'PermitRootLogin no' \
      'UseDNS no' \
      'AllowUsers tester' \
      'Subsystem sftp internal-sftp' \
      > /etc/ssh/sshd_config

EXPOSE 2222
CMD ["/usr/sbin/sshd", "-D", "-e"]

# Throwaway SSH target for testing sss. No systemd (so the persistence step is "not applicable" here).
ARG BASE=ubuntu:24.04
FROM ${BASE}
RUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
      openssh-server sudo ca-certificates && rm -rf /var/lib/apt/lists/* && mkdir -p /run/sshd
# tester: passwordless sudo.  plain: no sudo at all.
RUN useradd -m -s /bin/bash tester && echo 'tester ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/tester \
 && useradd -m -s /bin/bash plain
COPY keys/id_ed25519.pub /tmp/key.pub
RUN for u in tester plain; do mkdir -p /home/$u/.ssh && cp /tmp/key.pub /home/$u/.ssh/authorized_keys \
      && chown -R $u:$u /home/$u/.ssh && chmod 700 /home/$u/.ssh && chmod 600 /home/$u/.ssh/authorized_keys; done
EXPOSE 22
CMD ["/usr/sbin/sshd", "-D", "-e"]

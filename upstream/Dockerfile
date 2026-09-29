FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends openssh-server \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/sshd \
    && useradd -m -s /bin/bash proxyuser \
    && echo 'proxyuser:secret' | chpasswd

RUN printf '%s\n' \
    'PasswordAuthentication yes' \
    'PubkeyAuthentication yes' \
    'PermitRootLogin no' \
    > /etc/ssh/sshd_config.d/auth.conf

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 22
ENTRYPOINT ["/entrypoint.sh"]

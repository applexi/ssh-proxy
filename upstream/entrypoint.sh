#!/bin/sh
set -e

if [ -f /srv/keys/upstream_key.pub ]; then
    mkdir -p /home/proxyuser/.ssh
    cp /srv/keys/upstream_key.pub /home/proxyuser/.ssh/authorized_keys
    chown -R proxyuser:proxyuser /home/proxyuser/.ssh
    chmod 700 /home/proxyuser/.ssh
    chmod 600 /home/proxyuser/.ssh/authorized_keys
fi

ssh-keygen -A
exec /usr/sbin/sshd -D -e

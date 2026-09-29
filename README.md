# SSH proxy

## Overview

This project is a take-home assignment from Formal.

This directory implements a proxy, an intermediary between client users and an upstream server, using `golang.org/x/crypto/ssh`. Each client connection gets its own upstream connection. Each SSH `session` channel is paired with an upstream session channel. All of these are handled concurrently.

Shell traffic is forwarded as is to maximize transparency. Client session requests (`pty-req`, `shell`, `exec`, `window-change`) are forwarded upstream, and upstream requests (`exit-status`) are forwarded back. Stdin, stdout, and stderr are copied in both directions.

Client stdin is logged raw while those bytes are sent upstream. Each session gets `log_dir/<username>_<timestamp>.log`.

A session ends when upstream stdout and stderr both finish or when the proxy server is stopped. When a client user disconnects, their connection with the proxy and the respective upstream connection closes. The listener stays up for other clients as long as the proxy is open.

Authentication is based on a proxy [configuration file](#config-file-specifications). The proxy identifies itself to clients with `keys/proxy_host_key`.

## Config file specifications

Proxy refers to a configuration file path (default is `./config.yaml`). Please feel free to edit this file or create your own with the following specifications:

- Some fields need to be set to certain values for certain testing modes
- A Ed25519 proxy host key must exist in path `./keys/proxy_host_key`
- For the upstream server:
  - `server.username` is required
  - Authentication will check `server.password` and `server.private_key_file` if provided, but at least one is required
- For the client users:
  - At least one user must be provided
  - For each user, `user.username` is required
  - For each user, authentication will check `user.password` and `user.public_key` if provided, but at least one is required
- Session-specific client user input will be written to logs, stored in `log_dir`

## On your computer

Set `server.host` to `localhost`. Other fields will be based on the default [config file](./config.yaml).

First, start a Docker container to act as an upstream server.

```bash
docker run -d --name ssh-upstream -p 2222:2222 \
  -e USER_NAME=proxyuser \
  -e USER_PASSWORD=secret \
  -e PASSWORD_ACCESS=true \
  lscr.io/linuxserver/openssh-server
```

Then, in `formal-takehome`, start the proxy.

```bash
go run . -cfg config.yaml
```

In a separate terminal, you can now SSH into the upstream server through proxy.

```bash
ssh -i keys/alice_key -p 2022 alice@localhost
ssh -p 2022 bob@localhost
```

Try testing window change SSH requests with the following command.

```bash
ssh -t -p 2022 bob@localhost 'while true; do stty size; sleep 0.5; done'
```

## Docker testing

Set `server.host` to `upstream` and `server.port` to `22`.

```bash
docker compose up --build
```

From the host, the proxy is `localhost:2022` and the upstream server is `localhost:2222` (`proxyuser` / `secret`).

Compose assumes:

- `./config.yaml` is the proxy config file
- `log_dir` is `./logs`
- `./keys` contains `proxy_host_key`, `upstream_key`, and `upstream_key.pub`
- the upstream user in the config is `proxyuser` with password `secret`

# Deploying SupportSentinel on Alibaba Cloud (no Docker)

One ECS instance runs two systemd services; nginx terminates TLS and proxies to the API; PostgreSQL is Alibaba Cloud RDS. Re-deploys are one `make deploy`; this runbook covers the one-time setup.

## 0. Provision

- **ECS:** a small Linux instance (Ubuntu 22.04 / Alibaba Cloud Linux). Open security-group inbound ports **80** and **443** to the internet, **22** to your IP. Do NOT expose 8080/8090.
- **RDS:** an Alibaba Cloud RDS for PostgreSQL instance. Create a database `supportsentinel` and a user. Add the ECS instance's IP/VPC to the RDS whitelist. Note the connection string; managed RDS requires TLS (`sslmode=require`). The app applies its schema automatically on first connect (embedded `schema.sql`) — no manual migration.

## 1. Server user + directory

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin supportsentinel
sudo install -d -o "$USER" /opt/supportsentinel
sudo install -d /etc/supportsentinel
```

## 2. Environment file

Copy `deploy/app.env.prod.example` to the server, fill in `DATABASE_URL` (RDS) and `DASHSCOPE_API_KEY` (and optionally IMAP/SMTP/Slack), and install it locked down:

```bash
sudo install -m 600 -o supportsentinel app.env.prod /etc/supportsentinel/app.env
```

## 3. systemd units

```bash
sudo cp deploy/supportsentinel.service deploy/supportsentinel-mcp.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now supportsentinel-mcp.service supportsentinel.service
sudo systemctl --no-pager status supportsentinel.service
journalctl -u supportsentinel.service -n 50 --no-pager   # look for "listening on :8080"
```

## 4. Self-signed TLS + nginx

```bash
sudo install -d /etc/nginx/ssl
sudo openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout /etc/nginx/ssl/supportsentinel.key \
  -out    /etc/nginx/ssl/supportsentinel.crt \
  -subj "/CN=$(curl -s ifconfig.me)"
sudo cp deploy/nginx.conf /etc/nginx/conf.d/supportsentinel.conf
sudo nginx -t && sudo systemctl reload nginx
```

Visit `https://<ecs-public-ip>/` — accept the self-signed cert warning. The dashboard loads.

## 5. First binary deploy

From your laptop (repo root):

```bash
DEPLOY_HOST=<ecs-public-ip> DEPLOY_USER=<ssh-user> make deploy
```

This cross-compiles `server` + `mcp-server`, copies them to `/opt/supportsentinel/`, and restarts the services. Re-run it for every subsequent deploy.

## Verify

- `systemctl status supportsentinel.service supportsentinel-mcp.service` — both `active (running)`.
- `journalctl -u supportsentinel.service` shows `classifier: Qwen via DashScope (model=...) with tools (MCP http://127.0.0.1:8090/mcp)`.
- `curl -k https://<ip>/api/tickets` returns JSON.
- POST a ticket: `curl -k -X POST https://<ip>/api/emails -H 'content-type: application/json' -d '{"from":"a@b.com","subject":"test","body":"hello"}'`.

## Troubleshooting

- **Service crash-loops:** `journalctl -u supportsentinel.service -n 100` — usually a bad `DATABASE_URL` or missing `DASHSCOPE_API_KEY` (the latter falls back to the fake classifier, not a crash).
- **MCP fallback:** if `supportsentinel-mcp.service` is down, the API logs `mcp: dial ... failed; falling back to in-process tools` and still serves — tools just run in-process.
- **RDS connection refused:** check the RDS whitelist includes the ECS IP and that `sslmode=require` is set.

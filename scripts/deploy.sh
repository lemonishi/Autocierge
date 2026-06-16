#!/usr/bin/env bash
# Deploy Autocierge to an Alibaba Cloud ECS instance: cross-compile both
# binaries, copy them over, and restart the systemd services.
#
#   DEPLOY_HOST=<ecs-public-ip> DEPLOY_USER=<user> ./scripts/deploy.sh
#
# Assumes first-time setup (user, /opt/autocierge, /etc/autocierge/app.env,
# systemd units, nginx) is already done — see deploy/README.md.
set -euo pipefail

: "${DEPLOY_HOST:?set DEPLOY_HOST to the ECS public IP or hostname}"
DEPLOY_USER="${DEPLOY_USER:-root}"
REMOTE_DIR="/opt/autocierge"
SSH_TARGET="${DEPLOY_USER}@${DEPLOY_HOST}"

echo "==> building frontend + linux/amd64 binaries"
make build

echo "==> uploading binaries to ${SSH_TARGET}:${REMOTE_DIR}"
ssh "${SSH_TARGET}" "sudo install -d -o '${DEPLOY_USER}' '${REMOTE_DIR}'"
scp bin/server bin/mcp-server "${SSH_TARGET}:${REMOTE_DIR}/"

echo "==> restarting services (mcp first, then api)"
ssh "${SSH_TARGET}" "sudo systemctl restart autocierge-mcp.service autocierge.service && sudo systemctl --no-pager --lines=0 status autocierge.service"

echo "==> done — check: https://${DEPLOY_HOST}/  (self-signed cert warning is expected)"

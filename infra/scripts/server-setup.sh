#!/usr/bin/env bash
# One-time hardening + Docker install for a fresh Ubuntu 24.04+/Debian 12+ VPS.
# Run as root:  curl -fsSL <raw url> | bash   — or copy and run.
# Then: log in as `deploy`, clone the repo to ~/app, create infra/.env.prod, run deploy.sh.
set -euo pipefail

DEPLOY_USER=${DEPLOY_USER:-deploy}

apt-get update
apt-get -y upgrade
apt-get -y install ca-certificates curl ufw fail2ban unattended-upgrades git zstd age rclone

# Automatic security updates
dpkg-reconfigure -f noninteractive unattended-upgrades

# Docker Engine (official repo) — no Docker Desktop on servers
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/$(. /etc/os-release; echo "$ID")/gpg -o /etc/apt/keyrings/docker.asc
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/$(. /etc/os-release; echo "$ID") $(. /etc/os-release; echo "$VERSION_CODENAME") stable" \
  > /etc/apt/sources.list.d/docker.list
apt-get update
apt-get -y install docker-ce docker-ce-cli containerd.io docker-compose-plugin
cat >/etc/docker/daemon.json <<'JSON'
{ "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" }, "live-restore": true }
JSON
systemctl restart docker

# Deploy user (SSH key only)
id "$DEPLOY_USER" >/dev/null 2>&1 || adduser --disabled-password --gecos "" "$DEPLOY_USER"
usermod -aG docker "$DEPLOY_USER"
mkdir -p /home/$DEPLOY_USER/.ssh
[ -f /root/.ssh/authorized_keys ] && cp /root/.ssh/authorized_keys /home/$DEPLOY_USER/.ssh/
chown -R $DEPLOY_USER:$DEPLOY_USER /home/$DEPLOY_USER/.ssh && chmod 700 /home/$DEPLOY_USER/.ssh

# SSH: keys only, no root login
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/; s/^#\?PermitRootLogin.*/PermitRootLogin no/' /etc/ssh/sshd_config
systemctl reload ssh || systemctl reload sshd

# Firewall: only SSH + HTTP(S). Postgres is never exposed.
# NOTE: Docker-published ports bypass ufw; compose.prod.yml only publishes 80/443.
ufw default deny incoming
ufw default allow outgoing
ufw allow OpenSSH
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 443/udp
ufw --force enable

# 2 GB swap: protects small VPSes from OOM during video encoding
if ! swapon --show | grep -q swap; then
  fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
  sysctl -w vm.swappiness=10 && echo 'vm.swappiness=10' > /etc/sysctl.d/99-swappiness.conf
fi

echo "Done. Next: ssh $DEPLOY_USER@<server>, git clone <repo> ~/app, create ~/app/infra/.env.prod, run infra/scripts/deploy.sh"

#!/usr/bin/env bash
# One-time hardening + Docker install for a fresh Ubuntu 24.04 LTS VPS.
# Run as root:  curl -fsSL <raw url of this file> | bash -s -- <your-ssh-public-key>
set -euo pipefail
PUBKEY="${1:?usage: server-setup.sh '<ssh-ed25519 AAAA... you@laptop>'}"

apt-get update && apt-get -y upgrade
apt-get -y install ca-certificates curl ufw fail2ban unattended-upgrades rclone

# deploy user with your key, no password logins
id deploy >/dev/null 2>&1 || adduser --disabled-password --gecos "" deploy
install -d -m 700 -o deploy -g deploy /home/deploy/.ssh
echo "$PUBKEY" > /home/deploy/.ssh/authorized_keys
chown deploy:deploy /home/deploy/.ssh/authorized_keys && chmod 600 /home/deploy/.ssh/authorized_keys
sed -i -E 's/^#?PasswordAuthentication .*/PasswordAuthentication no/; s/^#?PermitRootLogin .*/PermitRootLogin no/' /etc/ssh/sshd_config
systemctl reload ssh

# firewall: only SSH + web
ufw default deny incoming && ufw default allow outgoing
ufw allow OpenSSH && ufw allow 80/tcp && ufw allow 443/tcp && ufw allow 443/udp
ufw --force enable

# automatic security updates
dpkg-reconfigure -f noninteractive unattended-upgrades

# swap: lets a 2 GB VPS survive video transcoding spikes
if [ ! -f /swapfile ]; then
  fallocate -l 2G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile
  echo '/swapfile none swap sw 0 0' >> /etc/fstab
  sysctl -w vm.swappiness=10 && echo 'vm.swappiness=10' > /etc/sysctl.d/99-swap.conf
fi

# Docker Engine (official script) + log rotation
curl -fsSL https://get.docker.com | sh
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<'JSON'
{ "log-driver": "json-file", "log-opts": { "max-size": "10m", "max-file": "3" }, "live-restore": true }
JSON
systemctl restart docker
usermod -aG docker deploy

install -d -o deploy -g deploy /opt/app
echo "Server ready. Next (as deploy): copy compose.prod.yaml, Caddyfile, backup.sh and .env to /opt/app – see docs/hosting.md"
# Note: Docker publishes ports past ufw rules; that is why only Caddy publishes ports in compose.prod.yaml.

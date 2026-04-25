#!/bin/bash
set -euxo pipefail

# Logs for troubleshooting cloud-init/user-data execution
exec > >(tee /var/log/user-data.log | logger -t user-data -s 2>/dev/console) 2>&1

# -----------------------------
# REQUIRED: set your repository here
# Example: REPO_URL="https://github.com/yourname/yourrepo.git"
# -----------------------------
REPO_URL="REPLACE_WITH_GIT_REPO_URL"
APP_DIR="/opt/devscale_social_media_app"
APP_BIN="devscale_social_media_app"
APP_USER="devscale"

install_packages() {
  if command -v dnf >/dev/null 2>&1; then
    dnf -y update
    dnf -y install golang git
  elif command -v yum >/dev/null 2>&1; then
    yum -y update
    yum -y install golang git
  elif command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y
    apt-get install -y golang-go git
  else
    echo "Unsupported OS: could not install Go/Git" >&2
    exit 1
  fi
}

if [ "$REPO_URL" = "REPLACE_WITH_GIT_REPO_URL" ]; then
  echo "Set REPO_URL in this script before using it." >&2
  exit 1
fi

install_packages

id -u "$APP_USER" >/dev/null 2>&1 || useradd --system --create-home --shell /sbin/nologin "$APP_USER"

rm -rf "$APP_DIR"
git clone "$REPO_URL" "$APP_DIR"

cd "$APP_DIR"
go build -o "$APP_BIN" .

chown -R "$APP_USER:$APP_USER" "$APP_DIR"

cat >/etc/systemd/system/devscale-socialmedia.service <<EOF
[Unit]
Description=DevScale Social Media Go App
After=network.target

[Service]
Type=simple
User=$APP_USER
Group=$APP_USER
WorkingDirectory=$APP_DIR
ExecStart=$APP_DIR/$APP_BIN
Environment=PORT=8080
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable devscale-socialmedia.service
systemctl restart devscale-socialmedia.service
systemctl is-active --quiet devscale-socialmedia.service

echo "Service is running. Test locally on instance: http://localhost:8080"
